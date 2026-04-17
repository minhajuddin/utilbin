package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

// reTmName mirrors the original tm script's `sed -E 's/[^a-z0-9_-]//g'`.
var reTmName = regexp.MustCompile(`[^a-z0-9_-]`)

func init() {
	root := &cobra.Command{
		Use:   "tm [name]",
		Short: "tmux session manager: open / list / switch / kill",
		Long: `tm without args opens or attaches a session named after the git branch
(when in a worktree) or the basename of the current directory. Pass a
name to open a specific session.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				n, err := tmDefaultSessionName()
				if err != nil {
					return err
				}
				name = n
			}
			if name == "" {
				return fmt.Errorf("session name is empty after sanitizing")
			}
			return tmOpen(name)
		},
	}

	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List tmux sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := run("tmux", "list-sessions"); err != nil {
				// tmux exits 1 if there are no sessions — print friendly msg.
				fmt.Fprintln(cmd.ErrOrStderr(), "no tmux sessions")
				return nil
			}
			return nil
		},
	}
	root.AddCommand(lsCmd)

	switchCmd := &cobra.Command{
		Use:   "switch",
		Short: "fzf-pick a session and switch/attach to it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := tmListNames()
			if err != nil {
				return err
			}
			if list == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "no tmux sessions")
				return nil
			}
			selected, err := fzfPick(list, "--prompt", "session> ")
			if err != nil {
				if errors.Is(err, errCanceled) {
					return nil
				}
				return err
			}
			return tmOpen(selected)
		},
	}
	root.AddCommand(switchCmd)

	killCmd := &cobra.Command{
		Use:   "kill [name]",
		Short: "Kill a session by name, or fzf-pick one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var target string
			if len(args) == 1 {
				target = args[0]
			} else {
				list, err := tmListNames()
				if err != nil {
					return err
				}
				if list == "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "no tmux sessions")
					return nil
				}
				selected, err := fzfPick(list, "--prompt", "kill session> ")
				if err != nil {
					if errors.Is(err, errCanceled) {
						return nil
					}
					return err
				}
				target = selected
			}
			return run("tmux", "kill-session", "-t", "="+target)
		},
	}
	root.AddCommand(killCmd)

	registry.Register("tm", nil, root)
}

// tmDefaultSessionName mirrors the original _tm_session_name: branch name
// inside a linked worktree, otherwise basename(pwd), with non
// [a-z0-9_-] stripped.
func tmDefaultSessionName() (string, error) {
	gitDir, _ := runOut("git", "rev-parse", "--git-dir")
	commonDir, _ := runOut("git", "rev-parse", "--git-common-dir")
	gitDir = strings.TrimSpace(gitDir)
	commonDir = strings.TrimSpace(commonDir)

	var name string
	if gitDir != "" && gitDir != commonDir {
		branch, err := runOut("git", "branch", "--show-current")
		if err != nil {
			return "", err
		}
		name = strings.TrimSpace(branch)
	} else {
		pwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		name = filepath.Base(pwd)
	}
	return reTmName.ReplaceAllString(name, ""), nil
}

// tmOpen attaches when outside tmux, switches client when already inside.
// `=name` makes tmux match the session name exactly.
func tmOpen(name string) error {
	target := "=" + name
	if exists, err := tmHasSession(name); err != nil {
		return err
	} else if exists {
		if os.Getenv("TMUX") != "" {
			return run("tmux", "switch-client", "-t", target)
		}
		return run("tmux", "attach-session", "-t", target)
	}
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return run("tmux", "new-session", "-s", name, "-c", pwd)
}

// tmHasSession returns true if a session by that exact name exists. tmux
// returns exit 1 when not found, which is not a real error here.
func tmHasSession(name string) (bool, error) {
	c := exec.Command("tmux", "has-session", "-t", "="+name)
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// tmListNames returns one session name per line, empty if no sessions.
func tmListNames() (string, error) {
	out, err := runOut("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// no sessions → tmux exits 1, treat as empty.
		return "", nil
	}
	return strings.TrimRight(out, "\n"), nil
}
