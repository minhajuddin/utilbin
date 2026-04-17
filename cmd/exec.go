package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// run shells out with the current process's stdio attached. Use for tools
// that take over the terminal (zellij, tmux, $EDITOR) or write
// human-readable output the user should see live.
func run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// runOut shells out and returns stdout as a string. stderr is forwarded to
// the user's stderr so error context isn't swallowed.
func runOut(name string, args ...string) (string, error) {
	var buf bytes.Buffer
	c := exec.Command(name, args...)
	c.Stdout = &buf
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// fzfPick pipes input lines into fzf and returns the selected line. Returns
// errCanceled if the user dismisses the picker (fzf exit 130). fzf draws its
// TUI on /dev/tty regardless of where we redirect its stdio.
func fzfPick(input string, fzfArgs ...string) (string, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return "", fmt.Errorf("fzf not found on PATH (install junegunn/fzf): %w", err)
	}
	args := append([]string{"--height", "40%", "--reverse"}, fzfArgs...)
	c := exec.Command("fzf", args...)
	c.Stdin = strings.NewReader(input)
	c.Stderr = os.Stderr
	out, err := c.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 130 {
			return "", errCanceled
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// errCanceled means the user dismissed an interactive picker. Commands
// should treat this as a clean exit, not an error.
var errCanceled = errors.New("user canceled")

// withInputPipe runs `cmd <stdin=input>` returning stdout. Like runOut but
// feeds stdin from a string.
func withInputPipe(input, name string, args ...string) (string, error) {
	var out bytes.Buffer
	c := exec.Command(name, args...)
	c.Stdin = strings.NewReader(input)
	c.Stdout = &out
	c.Stderr = os.Stderr
	err := c.Run()
	return out.String(), err
}

// firstField returns the first whitespace-delimited token of s. Useful for
// pulling a PR number out of a `gh pr list` line.
func firstField(s string) string {
	for _, f := range strings.Fields(s) {
		return f
	}
	return ""
}

// copyTo is a thin io.Copy wrapper that swallows the byte count.
func copyTo(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
}
