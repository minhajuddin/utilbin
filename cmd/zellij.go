package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	registerZ()
	registerZj()
	registerZz()
}

// reZjName allows lowercase alphanumerics, underscore, and hyphen — matches
// the sed filter used by the original zj script.
var reZjName = regexp.MustCompile(`[^a-z0-9_-]`)

func registerZ() {
	cmd := &cobra.Command{
		Use:   "z",
		Short: "Attach (or create) a zellij session named after the full pwd",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pwd, err := os.Getwd()
			if err != nil {
				return err
			}
			name := strings.ToLower(strings.ReplaceAll(pwd, "/", "-"))
			name = strings.Trim(name, "-")
			if name == "" {
				return fmt.Errorf("derived empty session name from pwd %q", pwd)
			}
			return run("zellij", "attach", "--create", name)
		},
	}
	registry.Register("z", nil, cmd)
}

func registerZj() {
	cmd := &cobra.Command{
		Use:   "zj [NAME]",
		Short: "Attach (or create) a zellij session named after basename(pwd) or NAME",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				pwd, err := os.Getwd()
				if err != nil {
					return err
				}
				name = reZjName.ReplaceAllString(filepath.Base(pwd), "")
			}
			if name == "" {
				return fmt.Errorf("session name is empty after sanitizing")
			}
			return run("zellij", "attach", "--create", name)
		},
	}
	registry.Register("zj", nil, cmd)
}

func registerZz() {
	cmd := &cobra.Command{
		Use:   "zz",
		Short: "fzf-pick a zellij session and attach to it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := runOut("zellij", "ls", "-n")
			if err != nil {
				return err
			}
			// Drop EXITED sessions and reduce to the first whitespace-token
			// (session name) per line — matches the original `grep -v EXITED
			// | cut -f1 -d' '` pipeline.
			var lines []string
			for line := range strings.SplitSeq(list, "\n") {
				if line == "" || strings.Contains(line, "EXITED") {
					continue
				}
				if name := firstField(line); name != "" {
					lines = append(lines, name)
				}
			}
			if len(lines) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no live zellij sessions")
				return nil
			}
			selected, err := fzfPick(strings.Join(lines, "\n"))
			if err != nil {
				if errors.Is(err, errCanceled) {
					return nil
				}
				return err
			}
			return run("zellij", "attach", "--create", selected)
		},
	}
	registry.Register("zz", nil, cmd)
}
