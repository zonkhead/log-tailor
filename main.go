package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	logger "log"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	logpb "cloud.google.com/go/logging/apiv2/loggingpb"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	// To get the proto defs
	_ "google.golang.org/genproto/googleapis/cloud/audit"
	_ "google.golang.org/genproto/googleapis/iam/v1/logging"
)

var config *Config

func main() {
	config = getConfig(parseArgs())

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *logpb.LogEntry)

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
func processLogEntries(wg *sync.WaitGroup, ch <-chan *logpb.LogEntry) {
	defer wg.Done()

	for {
		entry, ok := <-ch
		if !ok {
			break
		}
		if shouldDropEntry(entry) {
			continue
		}

		li := createLogItem(entry)

		switch config.Format {
		case "yaml":
			if bytes, err := yaml.Marshal(li); err != nil {
				stderrf("%v\n", err)
			} else {
				serialPrintf("---\n%+v", string(bytes))
			}
		case "json":
			if bytes, err := json.Marshal(li); err != nil {
				stderrf("%v\n", err)
			} else {
				serialPrintf("%+v\n", string(bytes))
			}
		case "csv":
			if om, ok := li.(OutputMap); ok {
				var row []string
				for k := range om {
					switch i := om[k].(type) {
					case string:
						row = append(row, i)
					case any:
						if bytes, err := json.Marshal(li); err != nil {
							stderrf("%v\n", err)
						} else {
							row = append(row, string(bytes))
						}
					}
				}
				serialCSVWrite(row)
			}
		}
	}
}

func createLogItem(entry *logpb.LogEntry) any {
	item := make(OutputMap)
	lname := logName(entry)

	for _, log := range config.Logs {
		if lname == log.Name {
			if log.ResType != "" && log.ResType != entry.Resource.Type {
				continue
			}
			if len(log.Output) > 0 {
				addOutputToItem(config.Common, item, entry)
				addOutputToItem(log.Output, item, entry)
				return item
			}
		}
	}

	return entry
}

func addOutputToItem(outputs []OutputMap, item OutputMap, entry *logpb.LogEntry) {
	for _, oi := range outputs {
		name := fieldName(oi)
		addToItem(name, oi[name], item, entry)
	}
}

func addToItem(name string, oi any, item OutputMap, entry *logpb.LogEntry) {
	if outItem, ok := oi.(OutputMap); ok {
		if hasKeys(outItem, "src", "regex", "value") {
			if src, ok := entryData(entry, strVal(outItem, "src")).(string); ok {
				rgx := strVal(outItem, "regex")
				val := strVal(outItem, "value")
				item[name] = regexVal(src, rgx, val)
			}
		} else {
			newItem := OutputMap{}
			item[name] = newItem
			for k := range outItem {
				addToItem(k, outItem[k], newItem, entry)
			}
		}
	} else {
		if v, ok := oi.(string); ok {
			item[name] = entryData(entry, v)
		}
	}
}

func fieldName(outItem OutputMap) string {
	name := ""
	for k := range outItem {
		name = k
	}
	return name
}

func logName(entry *logpb.LogEntry) string {
	re := regexp.MustCompile("^.*/(.*)$")
	m := re.FindStringSubmatch(entry.LogName)
	if m == nil {
		logger.Printf("Something is fundamentally broken with logging LogName: %s", entry.LogName)
		os.Exit(1)
	}
	fixed, _ := url.PathUnescape(m[1])
	return fixed
}

// Gets data from the LogEntry with a dot-separated path as a specifier.
// example path: resources.labels.project_id
func entryData(entry *logpb.LogEntry, path string) any {
	// Necessary hack for Golang naming of fields:
	switch path {
	case "logname":
		path = "LogName"
	case "receivetimestamp":
		path = "ReceiveTimestamp"
	}

	val := reflect.ValueOf(entry)

	for _, field := range pathElements(path) {
		// Dereference pointers if necessary
		if val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
			val = val.Elem()
		}

		switch val.Kind() {
		case reflect.Struct:
			val = val.FieldByName(capitalize(field))
			if !val.IsValid() {
				return fmt.Sprintf("Field %s not found", path)
			}
		case reflect.Map:
			// Access the map by key
			mapKey := reflect.ValueOf(field)
			val = val.MapIndex(mapKey)
			if !val.IsValid() {
				return fmt.Sprintf("Key %s not found in map", field)
			}
		case reflect.Ptr:
			// Handle protopayload
			if val.Type() == reflect.TypeOf(&logpb.LogEntry_ProtoPayload{}) {
				pp := val.Elem().Interface().(logpb.LogEntry_ProtoPayload)
				val = reflect.ValueOf(getProtoPayload(pp))
			}
		}
	}

	// Special handling for time.Time
	if val.Kind() == reflect.Ptr && val.Type() == reflect.TypeOf(&timestamppb.Timestamp{}) {
		ts := val.Elem().Interface().(timestamppb.Timestamp)
		return ts.AsTime().Format(time.RFC3339Nano)
	}

	// Hack for logname:
	if path == "LogName" {
		if s, ok := val.Interface().(string); ok {
			p, _ := url.PathUnescape(s)
			return p
		}
	}
	return val.Interface()
}

func getProtoPayload(pp logpb.LogEntry_ProtoPayload) any {
	typeURL := pp.ProtoPayload.TypeUrl
	messageName := getMessageNameFromTypeURL(typeURL)
	if messageName == "" {
		return fmt.Errorf("invalid type URL: %s", typeURL)
	}
	fullName := protoreflect.FullName(messageName)
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(fullName)
	if err != nil {
		return fmt.Errorf("message type not found: %v", err)
	}

	// Obtain the Message Descriptor from the Message Type
	desc := messageType.Descriptor()

	// Create a dynamic message based on the descriptor
	msg := dynamicpb.NewMessage(desc)

	// Unmarshal the value into the dynamic message
	if err := proto.Unmarshal(pp.ProtoPayload.Value, msg); err != nil {
		return fmt.Errorf("failed to unmarshal into dynamic message: %v", err)
	}

	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal dynamic message to JSON: %v", err)
	}

	var payloadMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &payloadMap)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal JSON to map: %v", err)
	}

	return payloadMap
}

func getMessageNameFromTypeURL(typeURL string) string {
	const prefix = "type.googleapis.com/"
	if !strings.HasPrefix(typeURL, prefix) {
		return ""
	}
	return strings.TrimPrefix(typeURL, prefix)
}

var printMU sync.Mutex

// Makes sure printing to stdout doesn't overlap.
func serialPrintf(format string, values ...any) {
	printMU.Lock()
	defer printMU.Unlock()
	fmt.Printf(format, values...)
}

// Makes sure printing to stdout doesn't overlap.
func serialCSVWrite(row []string) {
	printMU.Lock()
	defer printMU.Unlock()
	csvWr := csv.NewWriter(os.Stdout)
	csvWr.Write(row)
	csvWr.Flush()
}

// Determines if the entry should be logged to stdout
func shouldDropEntry(entry *logpb.LogEntry) bool {
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

func getProjID(entry *logpb.LogEntry) string {
	return entry.Resource.Labels["project_id"]
}

type TailClient logpb.LoggingServiceV2_TailLogEntriesClient

func startTailing(ctx context.Context, client *logging.Client, projID string) TailClient {
	stream, err := client.TailLogEntries(ctx)
	if err != nil {
		logger.Printf("Failed to start log entry tail: %v\n", err)
		os.Exit(1)
	}

	filter := createFilter(projID)

	req := &logpb.TailLogEntriesRequest{
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
func pullLogs(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, projID string, ch chan<- *logpb.LogEntry) {
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
			if config.Limit != NoLimit && pullCount >= config.Limit {
				stream.CloseSend()
				return
			}
			ch <- entry
			if config.Limit != NoLimit {
				pullCount++
				if pullCount == config.Limit {
					// We hit the limit. Make everyone un-block and go home.
					cancel()
					stream.CloseSend()
					return
				}
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
		return url.PathEscape(l)
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
