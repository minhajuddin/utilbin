package cmd

import (
	"io"
	"os"
	"strings"
)

// readInput is the standard input convention for text-encoding utilities:
// if positional args were supplied, join them with a space; otherwise read
// stdin and strip the trailing newline added by tools like `echo`.
func readInput(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

// readBinaryInput is like readInput but doesn't strip trailing whitespace.
// Use for decoders where every byte matters.
func readBinaryInput(args []string) ([]byte, error) {
	if len(args) > 0 {
		return []byte(strings.Join(args, " ")), nil
	}
	return io.ReadAll(os.Stdin)
}
