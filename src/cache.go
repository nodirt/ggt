package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// packageSnapshotCache stores previously ran benchmarks and known test names.
type packageSnapshotCache struct {
	Benchmarks benchmarkRunSlice
	// BenchmarksIsComplete is true if Benchmarks is a full list of benchmarks
	// in the package snapshot
	BenchmarksIsComplete bool

	AllBenchmarkNames []string // all test names. Nil if unknown.
}

// Load initializes c state from a file.
func (c *packageSnapshotCache) Load(filename string) error {
	*c = packageSnapshotCache{}
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(c)
}

// Save persists c state to a file.
func (c *packageSnapshotCache) Save(filename string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c)
}
