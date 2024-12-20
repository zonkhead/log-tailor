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

	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"
)

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
	projID  string
	format  string
	logs    stringList
	filters stringList
	limit   int
}

var args cmdlnArgs

func main() {
	parseArgs()
	projID := args.projID
	ctx := context.Background()

	client, err := logadmin.NewClient(ctx, projID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logadmin client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	var itr *logadmin.EntryIterator
	filter := createFilter()
	fmt.Println(filter)
	if filter != "" {
		itr = client.Entries(ctx, logadmin.Filter(filter))
	} else {
		itr = client.Entries(ctx)
	}

	for i := 0; i < args.limit; i++ {
		entry, err := itr.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error retrieving logs: %v\n", err)
			os.Exit(1)
		}

		var bytes []byte

		switch args.format {
		case "yaml":
			bytes, _ = yaml.Marshal(entry)
			fmt.Printf("---\n%+v\n", string(bytes))
		case "json":
			bytes, _ = json.Marshal(entry)
			fmt.Printf("%+v\n", string(bytes))
		}
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
	return fmt.Sprintf(`"projects/%s/logs/%s"`, args.projID, l)
}

func fixLogName(l string) string {
	if strings.Contains(l, "/") {
		return url.QueryEscape(l)
	}
	return l
}

func parseArgs() {
	flag.StringVar(&args.projID, "p", "", "Project ID")
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
