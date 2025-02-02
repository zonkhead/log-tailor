package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	logger "log"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

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

// Pulls log entries from the channel and prints them to stdout.
func processLogEntries(wg *sync.WaitGroup, ch <-chan *logpb.LogEntry) {
	wg.Add(1)
	defer wg.Done()

	for {
		entry, ok := <-ch

		if !ok {
			break
		}
		if shouldDropEntry(entry) {
			continue
		}

		li, match := createLogItem(entry)

		writer := bufio.NewWriter(os.Stdout)

		switch config.Format {
		case "yaml":
			processYAML(writer, li, match)
		case "jsonl":
			processJSON(writer, li)
		case "csv":
			processCSV(writer, li)
		}

		writer.Flush()

		if !config.Buffered {
			os.Stdout.Sync()
		}
	}
}

func processYAML(writer *bufio.Writer, li OutputMap, match *Log) {
	var logItem any = li

	if len(config.Common) > 0 || (match != nil && len(match.Output) > 0) {
		logItem = sortedYaml(li, match)
	}
	if bytes, err := yaml.Marshal(logItem); err != nil {
		stderrf("%v\n", err)
	} else {
		fmt.Fprintf(writer, "---\n%s", bytes)
	}
}

func processJSON(writer *bufio.Writer, li OutputMap) {
	if bytes, err := json.Marshal(li); err != nil {
		stderrf("%v\n", err)
	} else {
		fmt.Fprintf(writer, "%s\n", bytes)
	}
}

func processCSV(writer *bufio.Writer, li OutputMap) {
	var row []string
	if len(config.Common) > 0 {
		row = addOutputToRow(config.Common, li, row)
	}
	if len(config.Logs) > 0 {
		for _, l := range config.Logs {
			row = addOutputToRow(l.Output, li, row)
		}
	}
	serialCSVWrite(writer, row)
}

// Determines if the entry should be logged to stdout
func shouldDropEntry(entry *logpb.LogEntry) bool {
	if config.MatchRule == "drop-no-match" {
		for _, log := range config.Logs {
			lnMatch := logName(entry) == log.Name
			if !lnMatch {
				continue
			}
			typeMatch := true
			if log.ResType != "" {
				typeMatch = entry.Resource.Type == log.ResType
			}
			if typeMatch {
				return false
			}
		}
		return true
	}
	return false
}

func createLogItem(entry *logpb.LogEntry) (OutputMap, *Log) {
	item := make(OutputMap)
	lname := logName(entry)
	var match *Log

	addOutputToItem(config.Common, item, entry)

	// Find the first matching log and use its outputs
	for i := 0; i < len(config.Logs); i++ {
		log := &config.Logs[i]
		if lname == log.Name {
			if log.ResType != "" && log.ResType != entry.Resource.Type {
				continue
			}
			if len(log.Output) > 0 {
				addOutputToItem(log.Output, item, entry)
				match = log
				break
			}
		}
	}

	if len(item) == 0 {
		// There were no outputs specified so we use all the data
		addEntryToItem(item, entry)
	}
	return item, match
}

func addEntryToItem(item OutputMap, entry *logpb.LogEntry) {
	val := reflect.ValueOf(entry.Payload)

	switch entry.Payload.(type) {
	case *logpb.LogEntry_ProtoPayload:
		pp := val.Elem().Interface().(logpb.LogEntry_ProtoPayload)
		item["protoPayload"] = getProtoPayload(pp)

	case *logpb.LogEntry_JsonPayload:
		jp := val.Elem().Interface().(logpb.LogEntry_JsonPayload)
		item["jsonPayload"] = jp.JsonPayload.AsMap()

	case *logpb.LogEntry_TextPayload:
		tp := val.Elem().Interface().(logpb.LogEntry_TextPayload)
		item["textPayload"] = tp.TextPayload

	default:
		item["payload"] = entry.Payload
	}

	fixedLogName, _ := url.PathUnescape(entry.LogName)
	item["logName"] = fixedLogName
	if entry.Resource != nil {
		item["resource"] = entry.Resource
	}
	item["timestamp"] = entry.Timestamp.AsTime().Format(time.RFC3339Nano)
	item["receiveTimestamp"] = entry.ReceiveTimestamp.AsTime().Format(time.RFC3339Nano)
	item["severity"] = entry.Severity
	item["insertId"] = entry.InsertId
	if entry.HttpRequest != nil {
		item["httpRequest"] = entry.HttpRequest
	}
	item["labels"] = entry.Labels
	if entry.Operation != nil {
		item["operation"] = entry.Operation
	}
	item["trace"] = entry.Trace
	item["spanid"] = entry.SpanId
	item["tracesampled"] = entry.TraceSampled
	if entry.SourceLocation != nil {
		item["sourcelocation"] = entry.SourceLocation
	}
	if entry.Split != nil {
		item["split"] = entry.Split
	}
}

func addOutputToItem(outputs []OutputMap, item OutputMap, entry *logpb.LogEntry) {
	for _, oi := range outputs {
		name := fieldName(oi)
		addToItem(name, oi[name], item, entry)
	}
}

func addToItem(name string, oi any, item OutputMap, entry *logpb.LogEntry) {
	if om, ok := oi.(OutputMap); ok {
		if hasKeys(om, "src", "regex", "value") {
			if src, ok := entryData(entry, strVal(om, "src")).(string); ok {
				rgx := strVal(om, "regex")
				val := strVal(om, "value")
				item[name] = regexVal(src, rgx, val)
			}
		} else {
			newItem := OutputMap{}
			item[name] = newItem
			for k := range om {
				addToItem(k, om[k], newItem, entry)
			}
		}
	} else {
		if v, ok := oi.(string); ok {
			item[name] = entryData(entry, v)
		}
	}
}

func addOutputToRow(outputs []OutputMap, item OutputMap, row []string) []string {
	for _, m := range outputs {
		for k := range m {
			switch v := item[k].(type) {
			case string:
				row = append(row, v)
			default:
				bytes, err := json.Marshal(v)
				if err != nil {
					stderrf("Error marshaling key %s: %v\n", k, err)
					continue
				}
				row = append(row, string(bytes))
			}
		}
	}
	return row
}

func logName(entry *logpb.LogEntry) string {
	re := regexp.MustCompile("^.*/(.*)$")
	m := re.FindStringSubmatch(entry.LogName)
	if m == nil {
		logger.Printf("Something is fundamentally broken with logging LogName: %s", entry.LogName)
		return entry.LogName
	}
	fixed, _ := url.PathUnescape(m[1])
	return fixed
}

// Gets data from the LogEntry with a dot-separated path as a specifier.
// example path: resources.labels.project_id
func entryData(entry *logpb.LogEntry, path string) any {
	val := reflect.ValueOf(entry)

	for i, field := range pathElements(path) {
		// Dereference pointers if necessary
		if val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
			val = val.Elem()
		}

		// Compensate for cloud logging's GUIs transforming payload to different names
		if i == 0 && (field == "protoPayload" || field == "jsonPayload" || field == "textPayload") {
			if val.Type() == reflect.TypeOf(logpb.LogEntry{}) {
				val = reflect.ValueOf(entry.Payload)
			}
		}

		switch val.Kind() {
		case reflect.Ptr:
			switch p := val.Elem().Interface().(type) {
			case logpb.LogEntry_ProtoPayload:
				val = reflect.ValueOf(getProtoPayload(p))
				continue
			case logpb.LogEntry_JsonPayload:
				val = reflect.ValueOf(p.JsonPayload.AsMap())
				continue
			case logpb.LogEntry_TextPayload:
				val = reflect.ValueOf(p.TextPayload)
				continue
			}
		case reflect.Struct:
			val = val.FieldByName(capitalize(field))
			if !val.IsValid() {
				return fmt.Sprintf("Field %s not found", path)
			}
			// Special handling for time.Time
			if val.Type() == reflect.TypeOf(&timestamppb.Timestamp{}) {
				ts := val.Elem().Interface().(timestamppb.Timestamp)
				return ts.AsTime().Format(time.RFC3339Nano)
			}
		case reflect.Map:
			// Access the map by key
			mapKey := reflect.ValueOf(field)
			val = val.MapIndex(mapKey)
			if !val.IsValid() {
				return fmt.Sprintf("Key %s not found in map", field)
			}
		}
	}

	// Hack for logname:
	if path == "logName" {
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

	var payloadMap map[string]any
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
func serialCSVWrite(writer *bufio.Writer, row []string) {
	printMU.Lock()
	defer printMU.Unlock()
	csvWr := csv.NewWriter(writer)
	csvWr.Write(row)
	csvWr.Flush()
}
