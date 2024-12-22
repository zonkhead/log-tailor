package main

import (
	"flag"
	"math"
	"net/url"
	"strings"
)

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
