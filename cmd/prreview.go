package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

const prReviewMaxRetries = 3

var rePRURL = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/pull/([0-9]+)`)

func init() {
	var repoDir string

	cmd := &cobra.Command{
		Use:   "pr-review <github-pr-url>",
		Short: "Check out a PR in the local repo and open nvim DiffviewOpen against merge-base",
		Long: `pr-review fetches a PR, stashes any local work, checks out the PR
detached, then opens nvim with DiffviewOpen <merge-base>...HEAD against
the PR's base branch. On exit it restores your original branch and pops
the stash. Default --repo-dir is ~/carrot.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prURL := args[0]
			if !rePRURL.MatchString(prURL) {
				return fmt.Errorf("invalid GitHub PR URL (want https://github.com/owner/repo/pull/N): %s", prURL)
			}

			dir, err := expandHome(repoDir)
			if err != nil {
				return err
			}
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				return fmt.Errorf("repo dir %s does not exist", dir)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := os.Chdir(dir); err != nil {
				return err
			}
			defer os.Chdir(cwd)

			if _, err := runOut("git", "rev-parse", "--is-inside-work-tree"); err != nil {
				return fmt.Errorf("%s is not a git repository", dir)
			}

			fmt.Fprintln(cmd.ErrOrStderr(), "Fetching PR details...")
			meta, err := fetchPRRefs(prURL)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "PR branch: %s\nBase branch: %s\n", meta.HeadRefName, meta.BaseRefName)

			origBranch, err := runOut("git", "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return err
			}
			origBranch = strings.TrimSpace(origBranch)
			fmt.Fprintf(cmd.ErrOrStderr(), "Current branch: %s\n", origBranch)

			stashed := false
			if dirty, _ := workingTreeDirty(); dirty {
				fmt.Fprintln(cmd.ErrOrStderr(), "Uncommitted changes detected, stashing...")
				stashMsg := fmt.Sprintf("pr-review: auto-stash before reviewing %s", prURL)
				if err := withRetry(cmd, "stash changes", func() error {
					return run("git", "stash", "push", "-m", stashMsg)
				}); err != nil {
					return err
				}
				stashed = true
			}

			checkedOut := false
			defer func() {
				if checkedOut && origBranch != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "Cleaning up: returning to branch %q...\n", origBranch)
					_ = run("git", "checkout", origBranch)
				}
				if stashed {
					fmt.Fprintln(cmd.ErrOrStderr(), "Restoring stashed changes...")
					_ = run("git", "stash", "pop")
				}
			}()

			if err := withRetry(cmd, "checkout PR branch", func() error {
				return run("gh", "pr", "checkout", prURL, "--detach")
			}); err != nil {
				return err
			}
			checkedOut = true

			mergeBase, err := runOut("git", "merge-base", "HEAD", "origin/"+meta.BaseRefName)
			if err != nil {
				return err
			}
			mergeBase = strings.TrimSpace(mergeBase)

			fmt.Fprintln(cmd.ErrOrStderr(), "")
			fmt.Fprintln(cmd.ErrOrStderr(), "Opening neovim with diffview...")
			short := mergeBase
			if len(short) > 12 {
				short = short[:12]
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Comparing HEAD against merge-base: %s\n\n", short)

			return run("nvim", "-c", fmt.Sprintf("DiffviewOpen %s...HEAD", mergeBase))
		},
	}
	cmd.Flags().StringVar(&repoDir, "repo-dir", "~/carrot", "local checkout to review the PR in")
	registry.Register("pr-review", nil, cmd)
}

type prRefs struct {
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
}

func fetchPRRefs(url string) (prRefs, error) {
	var meta prRefs
	var lastErr error
	for attempt := 1; attempt <= prReviewMaxRetries; attempt++ {
		out, err := runOut("gh", "pr", "view", url, "--json", "headRefName,baseRefName")
		if err == nil {
			if err := json.Unmarshal([]byte(out), &meta); err != nil {
				return meta, fmt.Errorf("parse gh json: %w", err)
			}
			if meta.HeadRefName == "" {
				return meta, fmt.Errorf("could not determine PR branch name")
			}
			return meta, nil
		}
		lastErr = err
		if attempt < prReviewMaxRetries {
			time.Sleep(2 * time.Second)
		}
	}
	return meta, fmt.Errorf("fetch PR details failed after %d attempts: %w", prReviewMaxRetries, lastErr)
}

func workingTreeDirty() (bool, error) {
	if err := run("git", "diff", "--quiet"); err != nil {
		return true, nil
	}
	if err := run("git", "diff", "--cached", "--quiet"); err != nil {
		return true, nil
	}
	return false, nil
}

func withRetry(cmd *cobra.Command, label string, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= prReviewMaxRetries; attempt++ {
		fmt.Fprintf(cmd.ErrOrStderr(), "Attempting: %s (attempt %d/%d)...\n", label, attempt, prReviewMaxRetries)
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s failed on attempt %d\n", label, attempt)
		}
		if attempt < prReviewMaxRetries {
			fmt.Fprintln(cmd.ErrOrStderr(), "Retrying in 2 seconds...")
			time.Sleep(2 * time.Second)
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", label, prReviewMaxRetries, lastErr)
}

// expandHome resolves a leading ~/ to the user's home directory.
func expandHome(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
	}
	return p, nil
}
