package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	logger "log"
	"runtime"
	"strings"
	"sync"

	logging "cloud.google.com/go/logging/apiv2"
	logpb "cloud.google.com/go/logging/apiv2/loggingpb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	// To get the proto defs
	_ "google.golang.org/genproto/googleapis/cloud/audit"
	_ "google.golang.org/genproto/googleapis/iam/v1/logging"
)

var config *Config

const LogEntryChannelBufferSize int = 1024

func main() {
	stdin := readFromStdin()
	config = getConfig(stdin, parseArgs())

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *logpb.LogEntry, LogEntryChannelBufferSize)

	var pullWG sync.WaitGroup

	for _, p := range config.Projects {
		go pullLogs(ctx, cancel, &pullWG, p, ch)
	}

	numWorkers := 3 * runtime.NumCPU() // 3. Love it or leave it.
	var procWG sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		go processLogEntries(&procWG, ch)
	}

	pullWG.Wait()
	close(ch)

	procWG.Wait()
}

// Pulls log entries from cloud loggging and then puts them in the channel.
func pullLogs(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, projID string, ch chan<- *logpb.LogEntry) {
	wg.Add(1)
	defer wg.Done()

	stream := startTailing(ctx, projID)

	for {
		if stream == nil {
			return
		}

		resp, err := stream.Recv()

		if errors.Is(err, io.EOF) {
			logger.Printf("EOF: %s\n", projID)
			break
		}
		if errors.Is(err, context.Canceled) {
			break
		}

		if err != nil {
			stream.CloseSend()
			if isReconnectableGRPCError(err) {
				logger.Printf("Cloud Logging disconnected us (%s). Reconnecting...", projID)
				stream = startTailing(ctx, projID)
				continue
			}
			logger.Printf("Error receiving (%s):%T: %v. Disconnecting...", projID, err, err)
			return
		}

		for _, entry := range resp.Entries {
			if !putEntryIntoChannel(entry, ch, cancel) {
				stream.CloseSend()
				return
			}
		}
	}

	stream.CloseSend()
}

type TailClient logpb.LoggingServiceV2_TailLogEntriesClient

func startTailing(ctx context.Context, projID string) TailClient {
	client, err := logging.NewClient(ctx)

	if err != nil {
		logger.Printf("Failed to create logging client (%s): %v", projID, err)
		return nil
	}

	stream, err := client.TailLogEntries(ctx)
	if err != nil {
		logger.Printf("Failed to start log entry tail (%s): %v", projID, err)
		return nil
	}

	filter := createFilter(projID)

	req := &logpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + projID},
		Filter:        filter,
	}

	// Send the initial request to start streaming.
	if err := stream.Send(req); err != nil {
		logger.Printf("Failed to send tail request (%s): %v", projID, err)
		return nil
	}

	return stream
}

// If it's a reconnectable error, return true.
func isReconnectableGRPCError(err error) bool {
	if st, ok := status.FromError(err); ok {
		if st.Code() == codes.Unavailable ||
			st.Code() == codes.OutOfRange ||
			st.Code() == codes.Internal {
			return true
		}
	}
	return false
}

// Multiple goroutines share these.
var pullCount int = 0
var pcMU sync.Mutex

// Puts an entry into the channel unless we hit the limit. Hitting the limit returns false
// and cancels the channel if we hit the limit with this particular request.
func putEntryIntoChannel(entry *logpb.LogEntry, ch chan<- *logpb.LogEntry, cancel context.CancelFunc) bool {
	pcMU.Lock()
	defer pcMU.Unlock()
	if pullCount >= config.Limit {
		return false
	}

	ch <- entry
	pullCount++

	if pullCount == config.Limit {
		// We hit the limit. Make everyone un-block and go home.
		cancel()
		return false
	}

	return true
}

// Gets all the filters and logs and turns them into a string
func createFilter(proj string) string {
	var b strings.Builder
	b.WriteString(createLogsFilter(proj))
	filtersLen := len(config.Filters)
	if filtersLen > 0 {
		for _, f := range config.Filters {
			b.WriteString(" ")
			b.WriteString(f)
		}
	}
	return b.String()
}

func createLogsFilter(proj string) string {
	var b strings.Builder
	if len(config.Logs) > 0 {
		logs := logsToSet(config.Logs)
		b.WriteString("logName = (")
		for i, log := range logs {
			b.WriteString(`"` + toFQLogStr(proj, escLogName(log)) + `"`)
			if len(logs) > 1 && i < len(logs)-1 {
				b.WriteString(" OR ")
			}
		}
		b.WriteString(")")
	}
	return b.String()
}

func logsToSet(logs []Log) []string {
	set := make(map[string]*Log)
	for _, log := range logs {
		set[log.Name] = &log
	}
	var result []string
	for name := range set {
		result = append(result, name)
	}
	return result
}

func toFQLogStr(proj string, log string) string {
	return fmt.Sprintf(`projects/%s/logs/%s`, proj, log)
}
