package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	registerMb()
	registerGitChrono()
	registerX()
	registerNew()
	registerGitSub()
}

func registerMb() {
	cmd := &cobra.Command{
		Use:   "mb",
		Short: "Print git merge-base origin/master..HEAD",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := runOut("git", "merge-base", "origin/master", "HEAD")
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
	registry.Register("mb", nil, cmd)
}

func registerGitChrono() {
	cmd := &cobra.Command{
		Use:   "git-branch-chronologically-ordered",
		Short: "List local git branches by most recent commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run("git", "for-each-ref", "--sort=-committerdate",
				"refs/heads/",
				"--format=%(committerdate:short) %(refname:short) - %(committername)")
		},
	}
	registry.Register("git-branch-chronologically-ordered", []string{"gbco"}, cmd)
}

func registerX() {
	cmd := &cobra.Command{
		Use:   "x [MSG...]",
		Short: "Quick WIP commit + push current branch (defaults message to 'WIP')",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := "WIP"
			if len(args) > 0 {
				msg = strings.Join(args, " ")
			}
			if err := run("git", "commit", "-m", msg); err != nil {
				return err
			}
			branch, err := runOut("git", "branch", "--show-current")
			if err != nil {
				return err
			}
			branch = strings.TrimSpace(branch)
			if branch == "" {
				return fmt.Errorf("could not determine current branch")
			}
			return run("git", "push", "origin", branch)
		},
	}
	registry.Register("x", nil, cmd)
}

func registerNew() {
	var fromMain bool
	var prefix string

	cmd := &cobra.Command{
		Use:   "new <branch-suffix>",
		Short: "Branch from origin/<base> with a personal prefix (default: khaja/)",
		Long: `new fetches origin's main branch, stashes uncommitted work, and creates a
new branch off it named <prefix><suffix>. Defaults match the dot3/bin/new
script: prefix is "khaja/", base is auto-detected as "main" or "master".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			suffix := args[0]

			base := "main"
			if !fromMain {
				detected, err := detectMainBranch()
				if err != nil {
					return err
				}
				base = detected
			}

			if err := run("git", "fetch", "origin", base); err != nil {
				return err
			}
			// `git stash` exits 0 even when nothing to stash, so don't gate.
			_ = run("git", "stash")
			return run("git", "checkout", "origin/"+base, "-b", prefix+suffix)
		},
	}
	cmd.Flags().BoolVar(&fromMain, "main", false, "skip auto-detect; assume base branch is 'main'")
	cmd.Flags().StringVar(&prefix, "prefix", "khaja/", "branch name prefix")
	registry.Register("new", nil, cmd)
}

func detectMainBranch() (string, error) {
	out, err := runOut("git", "branch", "--list", "main", "master", "--no-color")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		line = strings.TrimSpace(line)
		if line == "main" || line == "master" {
			return line, nil
		}
	}
	return "", fmt.Errorf("neither 'main' nor 'master' branch found locally")
}

func registerGitSub() {
	var literal bool

	cmd := &cobra.Command{
		Use:   "git-sub OLD NEW",
		Short: "Find/replace OLD with NEW across files known to git",
		Long: `git-sub uses 'git grep -l OLD' to find files, then rewrites each one in
place. OLD is treated as a Go regular expression by default; pass --literal
for plain string substitution.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			old, new := args[0], args[1]

			out, err := runOut("git", "grep", "-l", old)
			if err != nil {
				// `git grep` exits 1 when no matches — that's not a script
				// failure, just nothing to do.
				if out == "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "no matching files")
					return nil
				}
				return err
			}

			var rx *regexp.Regexp
			if !literal {
				rx, err = regexp.Compile(old)
				if err != nil {
					return fmt.Errorf("invalid regex: %w", err)
				}
			}

			for path := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
				if path == "" {
					continue
				}
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var replaced []byte
				if literal {
					replaced = []byte(strings.ReplaceAll(string(body), old, new))
				} else {
					replaced = rx.ReplaceAll(body, []byte(new))
				}
				if err := os.WriteFile(path, replaced, info.Mode()); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "rewrote %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&literal, "literal", false, "treat OLD as a literal string, not a regex")
	registry.Register("git-sub", nil, cmd)
}
