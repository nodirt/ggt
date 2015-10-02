package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// commitTestRun stores result of a test run for a git commit.
type commitTestRun struct {
	benchmarks map[string]benchmarkRunSlice
	failed     *TestFailedError
}

type cmdLog struct {
	packages      []string
	benchRegex    string // will be passed to `go test`
	revisionRange string // will be passed to `git log`
	nsPerOpThreshold float64 // min abs NsPerOpChange to display
}

func (*cmdLog) name() string {
	return "log"
}

func (*cmdLog) shortDescription() string {
	return "git log with changed benchmark results"
}

func (*cmdLog) usage() {
	fmt.Println("usage: ggt log [options]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
}

// annotate computes nextRun.NsPerOpChange relative to r.
func (*cmdLog) annotate(r *commitTestRun, nextRun *benchmarkRun, relPackagePath string) {
	benchmarks, ok := r.benchmarks[relPackagePath]
	if !ok {
		return
	}
	prev := benchmarks.Find(nextRun.Name)
	if prev != nil {
		nextRun.Annotate(prev)
	}
}

func (l *cmdLog) parseFlags(args []string) error {
	flag.StringVar(&l.benchRegex, "bench", ".", "test name regex")
	flag.Float64Var(&l.nsPerOpThreshold, "threshold", 2.0, "minimum absolute ns/op change to display, in percents (0-100).")
	args = parseFlags(args)

	if l.nsPerOpThreshold < 0 || l.nsPerOpThreshold > 100 {
		return fmt.Errorf("threshold must be in [0, 100] interval")
	}

	if len(args) > 0 && args[0] != "--" {
		l.revisionRange = args[0]
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	l.packages = args
	if len(l.packages) == 0 {
		return fmt.Errorf("packages are not specified")
	}
	return nil
}

// cmdLog is `ggt log` command.
//
// Usage:
//    ggt log [options] [revision range] [--] [packages]
// Options:
//    -bench: same as -bench in `go test`
func (l *cmdLog) run() error {
	set, err := openPackageSet(l.packages)
	if err != nil {
		return err
	}

	logArgs := []string{"log", "--format=%H"}
	if l.revisionRange != "" {
		logArgs = append(logArgs, l.revisionRange)
	}
	gitLog := set.repo.git(logArgs...)
	gitLogStdout, err := gitLog.StdoutPipe()
	if err != nil {
		return err
	}
	gitLogReader := bufio.NewReader(gitLogStdout)
	logCmd(gitLog)
	if err := gitLog.Start(); err != nil {
		return fmt.Errorf("could not start git log: %s", err)
	}
	defer gitLog.Process.Kill()

	commitId, err := gitLogReader.ReadString('\n')
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	commitId = strings.TrimSuffix(commitId, "\n")

	// in each iteration, prevParentRun is test run results of parent of commit processed in prev iteration
	var prevParentRun *commitTestRun
	first := true
	for {
		parentCommitId, err := gitLogReader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		parentCommitId = strings.TrimSuffix(parentCommitId, "\n")

		// Print the commit.
		if first {
			first = false
		} else {
			fmt.Println()
		}
		printCommit := set.repo.git("log", "-1", commitId)
		printCommit.Stdout = os.Stdout
		logCmd(printCommit)
		if err = printCommit.Run(); err != nil {
			return err
		}
		fmt.Println()

		var parentRun *commitTestRun // test results of parent of current commit.
		if parentCommitId != "" {
			benchmarks, err := set.GetBenchmarks(parentCommitId, l.benchRegex, nil)
			if err != nil {
				if err, ok := err.(*TestFailedError); ok {
					parentRun = &commitTestRun{failed: err}
				}
				return err
			} else {
				parentRun = &commitTestRun{benchmarks: benchmarks}
			}
		}

		for _, p := range set.relPackagePaths {
			printBenchmark := func(b *benchmarkRun) {
				if parentRun != nil {
					l.annotate(parentRun, b, p)
					if float64(b.NsPerOpChange) < l.nsPerOpThreshold {
						return
					}
				}
				fmt.Println(b)
			}
			if prevParentRun != nil {
				// the commit we are processing in this iteration is parent of th commit
				// processed in the prev iteration. Thus prevParentRun contains results for
				// the commit we are processing in this iteration.
				for _, bs := range prevParentRun.benchmarks {
					for _, b := range bs {
						printBenchmark(&b)
					}
				}
			} else {
				if _, err := set.GetBenchmarks(commitId, l.benchRegex, printBenchmark); err != nil {
					if _, ok := err.(*TestFailedError); !ok {
						return err
					}
				}
			}
		}

		if parentCommitId == "" {
			// git log has completed.
			break
		}
		commitId = parentCommitId
		prevParentRun = parentRun
	}
	return nil
}
