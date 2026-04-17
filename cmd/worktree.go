package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	root := &cobra.Command{
		Use:   "wt",
		Short: "Git worktree manager (clone bare + add/rm/cd to worktrees)",
		Long: `wt manages a workflow where each branch lives in its own worktree
under ~/r/<repo>/<branch>/, with the bare repo at ~/r/<repo>/bare.git.

For 'wt cd' to actually change the parent shell's directory, source the
shim: eval "$(utilbin shell-init zsh)"`,
	}

	cloneCmd := &cobra.Command{
		Use:   "clone <url>",
		Short: "Clone <url> as a bare repo at ~/r/<repo>/bare.git",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			repoName := strings.TrimSuffix(filepath.Base(url), ".git")
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			target := filepath.Join(home, "r", repoName, "bare.git")
			if _, err := os.Stat(target); err == nil {
				return fmt.Errorf("%s already exists", target)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return run("git", "clone", "--bare", url, target)
		},
	}
	root.AddCommand(cloneCmd)

	cdCmd := &cobra.Command{
		Use:   "cd [pattern]",
		Short: "Print the path of the matching worktree (cd via shell shim)",
		Long: `Prints the absolute path of the worktree matching [pattern]; the shell
shim wraps this with `+"`cd \"$(utilbin wt cd \"$@\")\"`"+`.
With no pattern and multiple worktrees, opens fzf to pick one.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := ""
			if len(args) == 1 {
				pattern = args[0]
			}
			repoRoot, err := wtRepoRoot()
			if err != nil {
				return err
			}
			paths, err := wtList()
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				return fmt.Errorf("no worktrees found")
			}

			// Build display names relative to repoRoot to match the original
			// script's output, but keep the absolute path alongside.
			type wt struct{ name, path string }
			var items []wt
			for _, p := range paths {
				name := strings.TrimPrefix(p, repoRoot+"/")
				items = append(items, wt{name: name, path: p})
			}

			if pattern != "" {
				var filtered []wt
				for _, it := range items {
					if strings.Contains(it.name, pattern) {
						filtered = append(filtered, it)
					}
				}
				if len(filtered) == 0 {
					return fmt.Errorf("no worktrees matching %q", pattern)
				}
				items = filtered
			}

			if len(items) == 1 {
				fmt.Fprintln(cmd.OutOrStdout(), items[0].path)
				return nil
			}

			var lines []string
			for _, it := range items {
				lines = append(lines, it.name)
			}
			selected, err := fzfPick(strings.Join(lines, "\n"), "--prompt", "worktree> ")
			if err != nil {
				if errors.Is(err, errCanceled) {
					return nil
				}
				return err
			}
			for _, it := range items {
				if it.name == selected {
					fmt.Fprintln(cmd.OutOrStdout(), it.path)
					return nil
				}
			}
			return fmt.Errorf("internal: selection %q not in items", selected)
		},
	}
	root.AddCommand(cdCmd)

	addCmd := &cobra.Command{
		Use:   "add <branch-name>",
		Short: "Add a worktree at ~/r/<repo>/<branch> (creating the branch if needed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			repoRoot, err := wtRepoRoot()
			if err != nil {
				return err
			}
			target := filepath.Join(repoRoot, branch)
			// Try existing branch first; if that fails, create one. Mirrors
			// the original `git worktree add T B 2>/dev/null || git worktree add T -b B`.
			if err := run("git", "worktree", "add", target, branch); err != nil {
				return run("git", "worktree", "add", target, "-b", branch)
			}
			return nil
		},
	}
	root.AddCommand(addCmd)

	rmCmd := &cobra.Command{
		Use:   "rm <branch-name>",
		Short: "Remove the worktree at ~/r/<repo>/<branch> and prune",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			repoRoot, err := wtRepoRoot()
			if err != nil {
				return err
			}
			target := filepath.Join(repoRoot, branch)
			if _, err := os.Stat(target); err != nil {
				return fmt.Errorf("worktree %q not found at %s", branch, target)
			}
			if err := os.RemoveAll(target); err != nil {
				return err
			}
			if err := run("git", "worktree", "prune"); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed worktree %q\n", branch)
			return nil
		},
	}
	root.AddCommand(rmCmd)

	registry.Register("wt", nil, root)

	registerShellShim(wtPosixShim, wtFishShim)
}

// wtRepoRoot returns the parent directory of the bare/common git dir, which
// is where every worktree for this repo lives in our ~/r/<repo>/ layout.
func wtRepoRoot() (string, error) {
	out, err := runOut("git", "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not in a git worktree: %w", err)
	}
	common := strings.TrimSpace(out)
	abs, err := filepath.Abs(common)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// EvalSymlinks fails if the path doesn't exist as a real file;
		// fall back to the absolute form.
		resolved = abs
	}
	return filepath.Dir(resolved), nil
}

// wtList returns the absolute path of every worktree for the current repo,
// excluding the bare clone (which `git worktree list --porcelain` reports
// for some setups).
func wtList() ([]string, error) {
	out, err := runOut("git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		p := strings.TrimPrefix(line, "worktree ")
		if strings.HasSuffix(p, "/bare.git") {
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}

const wtPosixShim = `wt() {
    if [ $# -eq 0 ] || [ "$1" = "cd" ]; then
        [ "${1:-}" = "cd" ] && shift
        local __wt_target
        __wt_target=$(command utilbin wt cd "$@") || return $?
        [ -n "$__wt_target" ] && cd "$__wt_target"
        return
    fi
    command utilbin wt "$@"
}`

const wtFishShim = `function wt
    if test (count $argv) -eq 0; or test "$argv[1]" = "cd"
        if test (count $argv) -gt 0; and test "$argv[1]" = "cd"
            set argv $argv[2..-1]
        end
        set -l __wt_target (command utilbin wt cd $argv); or return $status
        if test -n "$__wt_target"
            cd "$__wt_target"
        end
        return
    end
    command utilbin wt $argv
end`
