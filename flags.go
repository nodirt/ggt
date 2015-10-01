package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"
)

var (
	verboseFlag bool // true to prints debug info
	colored     bool // false to disable colored output
	caching     bool // true to try to load test results from cache.
)

// verbose is a log.logger for verbose output.
var verbose = log.New(ioutil.Discard, "", 0)

// parseFlags wraps flag.CommandLine.Parse, restore "--" in args
// and processes common flags.
// Stops the process if flags cannot be parsed.
func parseFlags(args []string) []string {
	if err := flag.CommandLine.Parse(args); err != nil {
		usage()
		fatal(err)
	}
	args = restoreDashes(flag.Args())
	if verboseFlag {
		verbose = log.New(os.Stderr, "# ", 0)
	}
	return args
}

// restoreDashes restores "--" in args.
func restoreDashes(args []string) []string {
	if !containsString(args, "--") && containsString(os.Args, "--") {
		result := make([]string, 1, len(args)+1)
		result[0] = "--"
		args = append(result, args...)
	}
	return args
}

func init() {
	flag.BoolVar(&verboseFlag, "verbose", false, "print lots of stuff")
	flag.BoolVar(&colored, "colored", true, "print colored output")
	flag.BoolVar(&caching, "caching", true, "use on-disk cache for test results")
}

// containsString returns true if list contains elem.
func containsString(list []string, elem string) bool {
	for _, a := range list {
		if a == elem {
			return true
		}
	}
	return false
}
