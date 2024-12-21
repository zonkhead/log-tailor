package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"
)

func main() {
	parseArgs()
	ctx := context.Background()
	ch := make(chan *logging.Entry)

	for _, p := range args.projIDs {
		go pullLogs(ctx, p, ch)
	}

	for i := 0; i < args.limit; i++ {
		processLogEntry(<-ch)
	}
}

func processLogEntry(entry *logging.Entry) {
	var bytes []byte

	switch args.format {
	case "yaml":
		bytes, _ = yaml.Marshal(entry)
		fmt.Printf("---\n%+v", string(bytes))
	case "json":
		bytes, _ = json.Marshal(entry)
		fmt.Printf("%+v\n", string(bytes))
	}
}

func pullLogs(ctx context.Context, projID string, ch chan *logging.Entry) {
	client, err := logadmin.NewClient(ctx, projID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logadmin client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	var itr *logadmin.EntryIterator
	filter := createFilter()

	mostRecent := time.Now().UTC()
	// TODO: If it already contains a timestamp clause, don't mess with it.
	itr = getEntries(ctx, client, filter+" "+timestampFilter(mostRecent))

	for {
		entry, err := itr.Next()
		if err == iterator.Done {
			// Start polling becase we ran out of entries.
			time.Sleep(2 * time.Second)
			// TODO: This should really fix the existing filter in case it already
			//       contains a timestamp clause.
			itr = getEntries(ctx, client, filter+" "+timestampFilter(mostRecent))
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error retrieving logs (%s): %v\n", projID, err)
			return
		}
		mostRecent = time.Now().UTC()
		ch <- entry
	}
}

func timestampFilter(ts time.Time) string {
	return fmt.Sprintf(`timestamp >= "%s"`, ts.Format(time.RFC3339Nano))
}

func getEntries(ctx context.Context, client *logadmin.Client, filter string) *logadmin.EntryIterator {
	if filter != "" {
		return client.Entries(ctx, logadmin.Filter(filter))
	} else {
		return client.Entries(ctx)
	}
}

// Gets all the filters and logs and turns them into a string
func createFilter() string {
	var b strings.Builder
	b.WriteString(createLogsFilter())
	filtersLen := len(args.filters)
	if filtersLen > 0 {
		for _, f := range args.filters {
			b.WriteString(" ")
			b.WriteString(f)
		}
	}
	return b.String()
}

func createLogsFilter() string {
	var b strings.Builder
	logsLen := len(args.logs)
	if logsLen > 0 {
		b.WriteString("logName = (")
		for i, l := range args.logs {
			b.WriteString(logStr(l))
			if logsLen > 1 && i < logsLen-1 {
				b.WriteString(" OR ")
			}
		}
		b.WriteString(")")
	}
	return b.String()
}

func logStr(l string) string {
	return fmt.Sprintf(`"projects/%s/logs/%s"`, args.projIDs, l)
}

func fixLogName(l string) string {
	if strings.Contains(l, "/") {
		return url.QueryEscape(l)
	}
	return l
}

// ////
// For flag.Value support
type stringList []string

func (sl *stringList) String() string {
	return strings.Join(*sl, ",")
}

func (sl *stringList) Set(value string) error {
	*sl = append(*sl, value)
	return nil
}

// For flag.Value support
//////

type cmdlnArgs struct {
	projIDs stringList
	format  string
	logs    stringList
	filters stringList
	limit   int
}

var args cmdlnArgs

func parseArgs() {
	flag.Var(&args.projIDs, "p", "Project ID (multiple ok)")
	flag.StringVar(&args.format, "format", "yaml", "Format: json,yaml")
	flag.Var(&args.logs, "l", "Log to tail (short name, multiple ok)")
	flag.Var(&args.filters, "f", "Filter expression (multiple ok)")
	flag.IntVar(&args.limit, "limit", 0, "Number of entries to output. Defaults to 0 which is no-limit")
	flag.Parse()
	for i, l := range args.logs {
		args.logs[i] = fixLogName(l)
	}
	if args.limit == 0 {
		args.limit = math.MaxInt
	}
}
