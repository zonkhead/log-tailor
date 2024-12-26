package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	logger "log"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
)

var config *Config

func main() {
	config = getConfig(parseArgs())

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *loggingpb.LogEntry)

	var pullWG sync.WaitGroup

	for _, p := range config.Projects {
		pullWG.Add(1)
		go pullLogs(ctx, cancel, &pullWG, p, ch)
	}

	numWorkers := 3 * runtime.NumCPU() // 3. Love it or leave it.
	var procWG sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		procWG.Add(1)
		go processLogEntries(&procWG, ch)
	}

	pullWG.Wait()
	close(ch)

	procWG.Wait()
}

// Pulls log entries from the channel and prints them to stdout.
func processLogEntries(wg *sync.WaitGroup, ch <-chan *loggingpb.LogEntry) {
	defer wg.Done()
	for {
		entry, ok := <-ch
		if !ok {
			break
		}
		if shouldDropEntry(entry) {
			continue
		}

		var bytes []byte

		switch config.Format {
		case "yaml":
			bytes, _ = yaml.Marshal(entry)
			serialPrintf("---\n%+v", string(bytes))
		case "json":
			bytes, _ = json.Marshal(entry)
			serialPrintf("%+v\n", string(bytes))
		}
	}
}

var printMU sync.Mutex

// Makes sure printing to stdout doesn't overlap.
func serialPrintf(format string, values ...any) {
	printMU.Lock()
	defer printMU.Unlock()
	fmt.Printf(format, values...)
}

// Determines if the entry should be logged to stdout
func shouldDropEntry(entry *loggingpb.LogEntry) bool {
	if config.MatchRule == "drop-no-match" {
		for _, log := range config.Logs {
			lnMatch := entry.LogName == toLogStr(getProjID(entry), fixLogName(log.Name))
			typeMatch := true
			if log.ResType != "" {
				typeMatch = entry.Resource.Type == log.ResType
			}
			if lnMatch && typeMatch {
				return false
			}
		}
		return true
	}
	return false
}

func getProjID(entry *loggingpb.LogEntry) string {
	return entry.Resource.Labels["project_id"]
}

func startTailing(ctx context.Context, client *logging.Client, projID string) loggingpb.LoggingServiceV2_TailLogEntriesClient {
	stream, err := client.TailLogEntries(ctx)
	if err != nil {
		logger.Printf("Failed to start log entry tail: %v\n", err)
		os.Exit(1)
	}

	filter := createFilter(projID)

	req := &loggingpb.TailLogEntriesRequest{
		ResourceNames: []string{"projects/" + projID},
		Filter:        filter,
	}

	// Send the initial request to start streaming.
	if err := stream.Send(req); err != nil {
		logger.Printf("Failed to send tail request: %v\n", err)
		os.Exit(1)
	}

	return stream
}

// Multiple goroutines share these.
var pullCount int = 0
var pcMU sync.Mutex

// Pulls log entries from cloud loggging and then puts them in the channel.
func pullLogs(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, projID string, ch chan<- *loggingpb.LogEntry) {
	defer wg.Done()
	client, err := logging.NewClient(ctx)

	if err != nil {
		logger.Printf("Failed to create logging client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	stream := startTailing(ctx, client, projID)

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			logger.Printf("EOF: %s\n", projID)
			break
		}
		if err == context.Canceled {
			break
		}
		if err != nil {
			stream.CloseSend()
			logger.Printf("Error receiving (%s):%T: %v\n", projID, err, err)
			stream = startTailing(ctx, client, projID)
			continue
		}

		for _, entry := range resp.Entries {
			pcMU.Lock()
			if pullCount >= config.Limit {
				stream.CloseSend()
				return
			}
			ch <- entry
			pullCount++
			if pullCount == config.Limit {
				// We hit the limit. Make everyone un-block and go home.
				cancel()
				stream.CloseSend()
				return
			}
			pcMU.Unlock()
		}
	}

	stream.CloseSend()
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

// URL encodes the log name. Cloud logging is picky that way.
func fixLogName(l string) string {
	if strings.Contains(l, "/") {
		return url.QueryEscape(l)
	}
	return l
}

func createLogsFilter(proj string) string {
	var b strings.Builder
	logs := logsToSet(config.Logs)
	if len(logs) > 0 {
		b.WriteString("logName = (")
		for i, log := range logs {
			b.WriteString(`"` + toLogStr(proj, fixLogName(log)) + `"`)
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

func toLogStr(proj string, log string) string {
	return fmt.Sprintf(`projects/%s/logs/%s`, proj, log)
}
