package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// sandbox is able to checkout a repo at a revision to a temp dir
// and initialize PackageSnapshot.GoPath with it.
// Can be used to run tests on a revision different from HEAD.
type sandbox struct {
	packageSetSnapshot
	Revision string

	goPath string // GoPath that contains the package at Revision
}

func newSandbox(set *packageSet, revision string) (*sandbox, error) {
	treeId, err := trimOutput(set.repo.git("log", "-1", "--format=%T", revision))
	if err != nil {
		return nil, err
	}
	verbose.Printf("treeId of %s is %s\n", revision, treeId)

	s := &sandbox{
		packageSetSnapshot: *newPackageSetSnapshot(set, treeId),
		Revision:           revision,
	}
	s.InitGoPath = func() (string, error) {
		var err error
		if s.goPath == "" {
			err = s.Open()
		}
		return s.goPath, err
	}
	return s, nil
}

// Open checks out the repo at the revision to a temp dir.
func (s *sandbox) Open() error {
	if s.goPath != "" {
		return errors.New("sandbox already open")
	}
	goPath, err := ioutil.TempDir("", "ggt-")
	if err != nil {
		return err
	}

	checkout := filepath.Join(goPath, "src", s.rootPackageImportPath)
	if err := os.MkdirAll(checkout, os.ModePerm); err != nil {
		return err
	}
	verbose.Printf("sandboxing to %s...\n", checkout)
	gitCheckout := s.repo.git("--work-tree="+checkout, "checkout", s.Revision, "--", ".")
	logCmd(gitCheckout)
	if err = gitCheckout.Run(); err != nil {
		os.RemoveAll(goPath)
		return fmt.Errorf("could not checkout revision %s to %s: %s", s.Revision, checkout, err)
	}

	s.goPath = goPath
	return nil
}

// Close attempts to delete s.GoPath if present.
func (s *sandbox) Close() error {
	if s.goPath == "" {
		return nil
	}
	if err := os.RemoveAll(s.goPath); err != nil {
		fmt.Fprintf(os.Stderr, "could not delete checkout %s: %s\n", s.goPath, err)
		return err
	}
	s.goPath = ""
	return nil
}
