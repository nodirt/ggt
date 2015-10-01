package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

var benchmarkRunLineRegex = regexp.MustCompile(`^\s*(Benchmark[^\- ]*)(-\d+)?\s+(\d+)\s+(\d*(\.\d+)?) ns/op\s*$`)

// benchmarkRun contains number of iterations and speed.
type benchmarkRun struct {
	Line          string  // output of go test that this run was parsed from.
	Name          string  // test name
	N             int     // number of iterations
	NsPerOp       float32 // number of nanoseconds per iteration
	NsPerOpChange float32 // percentage of increase
}

// parseBenchmarkRun parses a BenchmarkRun from `go test` output line.
// Returns nil if cannot parse.
func parseBenchmarkRun(line string) *benchmarkRun {
	groups := benchmarkRunLineRegex.FindStringSubmatch(line)
	if len(groups) == 0 {
		return nil
	}

	n, err := strconv.Atoi(groups[3])
	if err != nil {
		panic(err)
	}

	nsop, err := strconv.ParseFloat(groups[4], 32)
	if err != nil {
		panic(err)
	}
	return &benchmarkRun{
		Line:    line,
		Name:    groups[1],
		N:       n,
		NsPerOp: float32(nsop),
	}
}

// Annotate computes r.NsPerOpChange compared to the prevRun.
// Returns true if found a matching benchmark in prevRun.
func (r *benchmarkRun) Annotate(prev *benchmarkRun) bool {
	if r.NsPerOp == prev.NsPerOp {
		r.NsPerOpChange = 0
	} else {
		r.NsPerOpChange = 100 * (r.NsPerOp - prev.NsPerOp) / prev.NsPerOp
	}
	return true
}

// String returns the original text output line, annotated with r.NsPerOpChange.
func (r benchmarkRun) String() string {
	result := r.Line
	if r.NsPerOpChange != 0 {
		deltaStr := fmt.Sprintf("%+f%%", r.NsPerOpChange)
		if colored {
			if r.NsPerOpChange > 0 {
				// more time is worse
				deltaStr = red(deltaStr)
			} else {
				deltaStr = green(deltaStr)
			}
		}
		result += "\t" + deltaStr
	}
	return result
}

// benchmarkRunSlice is a sorted slice of BenchmarkRun
type benchmarkRunSlice []benchmarkRun

func (s benchmarkRunSlice) Len() int {
	return len(s)
}

func (s benchmarkRunSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s benchmarkRunSlice) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (s benchmarkRunSlice) Search(name string) int {
	return sort.Search(
		len(s),
		func(i int) bool {
			return s[i].Name >= name
		})
}

// Find searches for a benchmark by name.
func (s benchmarkRunSlice) Find(name string) *benchmarkRun {
	i := s.Search(name)
	if i < len(s) && s[i].Name == name {
		return &s[i]
	}
	return nil
}

// Add inserts benchmark to the slice and keeps it sorted.
// Returns error if a benchmark of the same already exists in s.
func (s *benchmarkRunSlice) Add(benchmark *benchmarkRun) error {
	sv := *s
	i := s.Search(benchmark.Name)
	if i < len(sv) && sv[i].Name == benchmark.Name {
		return fmt.Errorf("Benchmark %s with this name is already present", benchmark.Name)
	}
	*s = append(sv[:i], append([]benchmarkRun{*benchmark}, sv[i:]...)...)
	return nil
}

type TestRun struct {
	Benchmarks benchmarkRunSlice
}
