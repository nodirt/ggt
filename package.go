package main

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
)

type repo struct {
	root   string // path to the repo root
	gitDir string // usually ".git"
}

// git creates a git command for the repo. Stderr is sent to os.Stderr
func (r *repo) git(args ...string) *exec.Cmd {
	return git(r.root, args...)
}

// packageSet is a collection of Go packages within one git repository.
type packageSet struct {
	repo
	rootPackageImportPath string   // import path of the root package in the repo
	relPackagePaths       []string // list of dirs relative to the root package
	packagesStrings       []string // packages specified on the command line, possibly patterns.
}

// goListEntry is one of packages returned by `go list`.
type goListEntry struct {
	dir        string
	importPath string
}

// resolvePackages returns importPath and location for each package.
// packages parameter may contain patterns.
func resolvePackages(packages []string) ([]goListEntry, error) {
	args := append([]string{"list", "-f", "{{.Dir}}:{{.ImportPath}}"}, packages...)
	out, err := trimOutput(exec.Command("go", args...))
	var result []goListEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\n")
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			log.Panicf("unexpected go list output: %s", line)
		}
		result = append(result, goListEntry{parts[0], parts[1]})
	}
	if err != nil {
		return nil, fmt.Errorf("cannot resolve packages %s: %s", packages, err)
	}
	return result, nil
}

func openPackageSet(packages []string) (*packageSet, error) {
	entries, err := resolvePackages(packages)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("packages not found")
	}
	for _, e := range entries {
		if e.importPath == "_"+e.dir {
			return nil, fmt.Errorf("package %s is not under a $GOPATH or $GOROOT", e.dir)
		}
	}
	verbose.Printf("resolved packages: %s\n", entries)

	set := packageSet{
		packagesStrings: packages[:],
		relPackagePaths: make([]string, len(entries)),
	}
	for i, e := range entries {
		gitDir, err := trimOutput(git(e.dir, "rev-parse", "--git-dir")) // may return relative path
		if err != nil {
			return nil, fmt.Errorf("package %s is not in a git repository: %s", e.dir, err)
		}
		// make gitDir absolute
		gitDir = filepath.Join(e.dir, gitDir)
		repoRoot := filepath.Dir(gitDir)
		set.relPackagePaths[i], err = filepath.Rel(repoRoot, e.dir)
		if err != nil {
			return nil, err
		}
		if set.root == "" {
			set.root = repoRoot
			set.gitDir = filepath.Base(gitDir)
			set.rootPackageImportPath = e.importPath
			relPath := set.relPackagePaths[i]
			if relPath != "." && !strings.HasPrefix(relPath, "./") {
				panic("relative path does not start with './'")
			}
			for relPath != "." {
				set.rootPackageImportPath = filepath.Dir(set.rootPackageImportPath)
				relPath = filepath.Dir(relPath)
			}
		} else if set.root != repoRoot {
			return nil, fmt.Errorf("packages span multiple git repositories")
		}
	}
	return &set, nil
}

// GetBenchmarks returns a mapping {packageImportPath -> benchmarks} at revision.
// cb is called on each benchmark as soon as it is received.
func (s *packageSet) GetBenchmarks(revision, benchRegex string, cb func(*benchmarkRun)) (map[string]benchmarkRunSlice, error) {
	sandbox, err := newSandbox(s, revision)
	if err != nil {
		return nil, err
	}
	defer sandbox.Close()
	return sandbox.GetBenchmarks(benchRegex, cb)
}
