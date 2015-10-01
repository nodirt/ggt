package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func usage() {
	fmt.Println("usage: ggt [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("\tlog: git log with benchmark results and deltas")
	fmt.Println()
	fmt.Println("Global options:")
	flag.PrintDefaults()
}

func main() {
	args := os.Args
	if len(args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := args[1]
	var err error
	switch cmd {
	case "log":
		err = new(cmdLog).run(args[2:])
	default:
		if strings.HasPrefix(cmd, "-") {
			fmt.Fprintln(os.Stderr, "command not specified")
		} else {
			fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		}
		usage()
		os.Exit(1)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(a ...interface{}) {
	msg := fmt.Sprint(a...)
	if colored {
		msg = red(msg)
	}
	log.Fatal(msg)
}
