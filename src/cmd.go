package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"bufio"

	"github.com/fatih/color"
)

var (
	red   = color.New(color.FgRed).SprintFunc()
	green = color.New(color.FgGreen).SprintFunc()
)

// trimOutput runs the command and returns its stdout output with trimmed whitespace.
// if cmd.Stderr is not set, it is set to redStderr.
func trimOutput(cmd *exec.Cmd) (string, error) {
	if cmd.Stderr == nil {
		cmd.Stderr = redStderr
	}
	logCmd(cmd)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// logCmd prints the command to verbose logger.
func logCmd(cmd *exec.Cmd) {
	if cmd.Dir != "" {
		panic("we don't specify cmd.Dir")
	}
	var buf bytes.Buffer
	buf.WriteString("$ ")
	for _, e := range cmd.Env {
		buf.WriteString(strings.TrimSpace(e))
		buf.WriteString(" ")
	}
	buf.WriteString(strings.Join(cmd.Args, " "))
	verbose.Println(buf.String())
}

type lineReader interface {
	ReadString(delim byte) (string, error)
}

// forEachLine calls f for each line in .
// line in f may have "\n"suffix.
func forEachLine(r lineReader, f func(line string) error) error {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if err := f(line); err != nil {
			return err
		}
	}
	return nil
}

// forEachLineOutput runs cmd and invokes f for each line in stdout.
// line in f may have "\n" suffix.
func forEachLineOutput(cmd *exec.Cmd, f func(line string) error) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdoutReader := bufio.NewReader(stdout)

	var lineProcessingErr error
	go func() {
		lineProcessingErr = forEachLine(stdoutReader, f)
	}()
	logCmd(cmd)
	err = cmd.Run()
	if err == nil {
		err = lineProcessingErr
	}
	return err
}

type writerFunc func([]byte) (n int, err error)

func (f writerFunc) Write(data []byte) (n int, err error) {
	return f(data)
}

// git runs git in the repo at repoPath.
// Redirects stderr to redStderr
func git(repoPath string, args ...string) *exec.Cmd {
	firstArgs := []string{"-C", repoPath}
	cmd := exec.Command("git", append(firstArgs, args...)...)
	cmd.Stderr = redStderr
	return cmd
}

// redStderr is an io.Writer that writes to os.Stderr in red color (unless -colored=false).
var redStderr = writerFunc(func(data []byte) (n int, err error) {
	text := string(data)
	if colored {
		text = red(text)
	}
	os.Stderr.WriteString(text)
	return len(data), nil
})
