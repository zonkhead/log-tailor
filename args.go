package main

import (
	"flag"
	"math"
	"os"
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
	projIDs  stringList
	format   string
	logs     stringList
	filters  stringList
	limit    int
	buffered bool
}

var _args cmdlnArgs

func parseArgs() *cmdlnArgs {
	flag.Var(&_args.projIDs, "p", "Project ID (multiple ok)")
	flag.StringVar(&_args.format, "format", "yaml", "Format: jsonl,yaml,csv")
	flag.Var(&_args.logs, "l", "Log to tail (short name, multiple ok)")
	flag.Var(&_args.filters, "f", "Filter expression (multiple ok)")
	flag.IntVar(&_args.limit, "limit", math.MaxInt, "Number of entries to output.")
	flag.BoolVar(&_args.buffered, "buffered", false, "Buffered stdout")
	version := flag.Bool("version", false, "Show version info")

	flag.Usage = func() {
		stderrln("Usage of log-tailor:")
		stderrln("  An application that tails GCP Cloud Logging and lets you ")
		stderrln("  customize the output. See the README for details:")
		stderrln("  https://github.com/zonkhead/log-tailor\n")
		stderrln("Options:")
		flag.PrintDefaults()
	}

	flag.Parse()
	if *version {
		stderrln("Version: 0.3.2")
		os.Exit(0)
	}
	switch _args.format {
	case "jsonl":
	case "yaml":
	case "csv":
	default:
		stderrln("Invalid format: " + _args.format)
		os.Exit(1)
	}
	if _args.limit <= 0 {
		_args.limit = math.MaxInt
	}
	return &_args
}
