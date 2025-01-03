package main

import (
	"flag"
	"strings"
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
	projIDs stringList
	format  string
	logs    stringList
	filters stringList
	limit   int
}

var NoLimit int = -1

var _args cmdlnArgs

func parseArgs() *cmdlnArgs {
	flag.Var(&_args.projIDs, "p", "Project ID (multiple ok)")
	flag.StringVar(&_args.format, "format", "yaml", "Format: json,yaml,csv")
	flag.Var(&_args.logs, "l", "Log to tail (short name, multiple ok)")
	flag.Var(&_args.filters, "f", "Filter expression (multiple ok)")
	flag.IntVar(&_args.limit, "limit", NoLimit, "Number of entries to output. Defaults to no-limit")
	flag.Parse()
	return &_args
}
