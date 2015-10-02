package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

type command interface {
	name() string
	shortDescription() string
	parseFlags(args []string) error
	run() error
	usage()
}

var commands = map[string]command {
	"cmd": &cmdLog{},
}

func usage() {
	fmt.Println("usage: ggt [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, c := range commands {
		fmt.Printf("\t%s: %s\n", c.name(), c.shortDescription())
	}
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

	cmdName :=args[1]
	var cmd command
	for _, c := range commands {
		if c.name() == cmdName {
			cmd = c
			break
		}
	}
	if cmd == nil {
		if !strings.HasPrefix(cmdName, "-") {
			fmt.Fprintln(os.Stderr, "unknown command:", cmdName)
		}
		usage()
		os.Exit(1)
	}

	if err := cmd.parseFlags(args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		cmd.usage()
		os.Exit(1)
	}

	if err := cmd.run(); err != nil {
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
