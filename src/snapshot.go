package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"io"
)

type TestFailedError struct {
	Exit         *exec.ExitError
	StderrOutput []byte
}

func (e *TestFailedError) Error() string {
	return "test failed"
}

type packageSetSnapshot struct {
	*packageSet

	Packages []packageSnapshot
	TreeId   string

	InitGoPath func() (string, error)
	GoPath     string
}

func newPackageSetSnapshot(set *packageSet, treeId string) *packageSetSnapshot {
	snapshot := packageSetSnapshot{
		packageSet: set,
		Packages:   make([]packageSnapshot, len(set.relPackagePaths)),
		TreeId:     treeId,
	}
	for i, rpp := range set.relPackagePaths {
		snapshot.Packages[i] = packageSnapshot{
			relPackagePath: rpp,
			PackageSet:     &snapshot,
		}
	}
	return &snapshot
}

type packageSnapshot struct {
	relPackagePath string
	PackageSet     *packageSetSnapshot
	Cache          *packageSnapshotCache
}

// Runs go command in the repo snapshot.
// Redirects stderr to current redStderr.
func (s *packageSetSnapshot) Go(args ...string) (*exec.Cmd, error) {
	cmd := exec.Command("go", args...)
	if s.GoPath == "" && s.InitGoPath != nil {
		var err error
		if s.GoPath, err = s.InitGoPath(); err != nil {
			return nil, err
		}
	}
	if s.GoPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GOPATH=%s:%s", s.GoPath, os.Getenv("GOPATH")))
	}
	cmd.Stderr = redStderr
	return cmd, nil
}

// GetBenchmarks returns a mapping {relPackagePath -> benchmarks}
// cb is called on each benchmark as soon as it is received.
func (s *packageSetSnapshot) GetBenchmarks(benchRegex string, cb func(*benchmarkRun)) (map[string]benchmarkRunSlice, error) {
	results := make(map[string]benchmarkRunSlice, len(s.Packages))
	for i := range s.Packages {
		p := &s.Packages[i]
		var err error
		results[p.relPackagePath], err = p.GetBenchmarks(benchRegex, cb)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// cacheFilename returns path to the snapshot cache file.
func (s *packageSnapshot) cacheFilename() string {
	return filepath.Join(
		s.PackageSet.repo.root,
		s.PackageSet.repo.gitDir,
		"ggt",
		"tree-cache",
		s.PackageSet.TreeId,
		s.relPackagePath,
		"dir-cache.json")
}

// LoadCache loads s.Cache from the cache file.
func (s *packageSnapshot) LoadCache() {
	if s.Cache == nil {
		s.Cache = &packageSnapshotCache{}
	}
	if err := s.Cache.Load(s.cacheFilename()); err != nil {
		log.Printf("could not load cache: %s\n", err)
	}
}

// LoadCache loads s.Cache from the cache file if it was not loaded before.
func (s *packageSnapshot) EnsureCacheLoaded() {
	if s.Cache == nil {
		s.LoadCache()
	}
}

// SaveCache saves s.Cache to the cache file.
func (s *packageSnapshot) SaveCache() {
	if s.Cache == nil {
		panic("cache not loaded")
	}
	if err := s.Cache.Save(s.cacheFilename()); err != nil {
		log.Printf("could not save test results: %s\n", err)
	}
}

// GetBenchmarkNames returns a slice of all benchmark names in the snapshot.
func (s *packageSnapshot) GetBenchmarkNames() ([]string, error) {
	s.EnsureCacheLoaded()

	if s.Cache.AllBenchmarkNames != nil {
		return s.Cache.AllBenchmarkNames, nil
	}

	importPath := filepath.Join(s.PackageSet.rootPackageImportPath, s.relPackagePath)
	test, err := s.PackageSet.Go("test", "-run=@", "-bench=.", "-benchtime=0", importPath)
	if err != nil {
		return nil, err
	}
	out, err := trimOutput(test)
	if err != nil {
		return nil, err
	}
	testNames := []string{} // must be non-nil
	for _, line := range strings.Split(out, "\n") {
		benchmark := parseBenchmarkRun(line)
		if benchmark != nil {
			testNames = append(testNames, benchmark.Name)
		}
	}

	s.Cache.AllBenchmarkNames = testNames
	s.SaveCache()
	return testNames, nil
}

// RunBenchmarks runs `go test -run=@ -bench=<benchRegex>` and returns parsed benchmarks.
// if benchRegex is "", it is defaulted to ".".
func (s *packageSnapshot) RunBenchmarks(benchRegex string, cb func(*benchmarkRun)) (benchmarkRunSlice, error) {
	s.EnsureCacheLoaded()
	if benchRegex == "" {
		benchRegex = "."
	}

	importPath := filepath.Join(s.PackageSet.rootPackageImportPath, s.relPackagePath)
	test, err := s.PackageSet.Go("test", "-run=@", "-bench="+benchRegex, importPath)
	if err != nil {
		return nil, err
	}
	testNames := []string{} // must be non-nil
	var result benchmarkRunSlice

	var stderr bytes.Buffer
	test.Stderr = io.MultiWriter(redStderr, &stderr)
	err = forEachLineOutput(test, func(line string) error {
		verbose.Print("\t", line)
		benchmark := parseBenchmarkRun(line)
		if benchmark == nil {
			return nil
		}
		verbose.Println("this is a benchmark")
		if cb != nil {
			cb(benchmark)
		}
		if err = result.Add(benchmark); err != nil {
			return err
		}
		if err = s.Cache.Benchmarks.Add(benchmark); err != nil {
			return err
		}
		testNames = append(testNames, benchmark.Name)
		return nil
	})
	if err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			return nil, &TestFailedError{err, stderr.Bytes()}
		}
		return nil, err
	}

	if benchRegex == "." {
		s.Cache.BenchmarksIsComplete = true
		s.Cache.AllBenchmarkNames = testNames
	}
	s.SaveCache()

	return result, nil
}

// loadBenchmarksFromCache attempts to load benchmarks from cache. If some benchmarks are missing, runs them.
func (s *packageSnapshot) loadBenchmarksFromCache(benchRegex string, cb func(*benchmarkRun)) (benchmarkRunSlice, error) {
	s.EnsureCacheLoaded()

	if len(s.Cache.Benchmarks) == 0 {
		verbose.Println("nothing in cache")
		return nil, nil
	}

	compiledBenchRegex, err := regexp.Compile(benchRegex)
	if err != nil {
		return nil, fmt.Errorf("invalid regexp: %s", benchRegex)
	}

	var result benchmarkRunSlice
	verbose.Printf("benchmarks in cache: %s\n", s.Cache.Benchmarks)
	for i := range s.Cache.Benchmarks {
		b := &s.Cache.Benchmarks[i]
		if compiledBenchRegex.MatchString(b.Name) {
			if cb != nil {
				cb(b)
			}
			if err := result.Add(b); err != nil {
				return nil, err
			}
		}
	}

	if !s.Cache.BenchmarksIsComplete {
		verbose.Println("not all tests are in cache. Getting full benchmark name list.")
		all, err := s.GetBenchmarkNames()
		if err != nil {
			return nil, err
		}
		var missing []string
		for _, t := range all {
			if !compiledBenchRegex.MatchString(t) {
				continue
			}
			if result.Find(t) == nil {
				missing = append(missing, t)
			}
		}
		if len(missing) > 0 {
			verbose.Printf("the benchmarks loaded from cache miss requested tests: %s.\n", missing)
			missingRgx := "^(" + strings.Join(missing, "|") + ")$"
			missingBenchmarks, err := s.RunBenchmarks(missingRgx, cb)
			if err != nil {
				return nil, err
			}
			for _, name := range missing {
				b := missingBenchmarks.Find(name)
				if b == nil {
					return nil, fmt.Errorf("requested benchmark %s didn't run.", name)
				}
				if err := result.Add(b); err != nil {
					return nil, err
				}
			}
		}
	}
	return result, nil
}

// GetBenchmarks returns benchmarks from cache or by running them.
// cb is called as soon as a benchmark is available.
// benchRegex is defaulted to "."
func (s *packageSnapshot) GetBenchmarks(benchRegex string, cb func(*benchmarkRun)) (benchmarkRunSlice, error) {
	if cb == nil {
		cb = func(*benchmarkRun) {}
	}

	if caching {
		benchmarks, err := s.loadBenchmarksFromCache(benchRegex, cb)
		if err != nil || benchmarks != nil {
			return benchmarks, err
		}
	}

	return s.RunBenchmarks(benchRegex, cb)
}
