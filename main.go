package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"regexp"
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
	if !hasTimestampClause(filter) {
		filter = setTimestampClause(filter, mostRecent)
	}
	itr = getEntries(ctx, client, filter)

	for {
		entry, err := itr.Next()
		if err == iterator.Done {
			// Start polling becase we ran out of entries.
			time.Sleep(2 * time.Second)
			itr = getEntries(ctx, client, setTimestampClause(filter, mostRecent))
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

func hasTimestampClause(f string) bool {
	regex := `timestamp\s*([><=]+)\s*"(.*?)"`
	re := regexp.MustCompile(regex)
	return re.MatchString(f)
}

func setTimestampClause(f string, ts time.Time) string {
	if hasTimestampClause(f) {
		regex := `timestamp\s*([><=]+)\s*"(.*?)"`
		re := regexp.MustCompile(regex)
		return re.ReplaceAllString(f, timestampFilter(ts))
	} else {
		return f + " " + timestampFilter(ts)
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
