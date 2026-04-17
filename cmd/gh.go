package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

const defaultPRRepo = "instacart/carrot"

func init() {
	registerCo()
	registerWip()
	registerGhPRPreview()
}

func registerCo() {
	cmd := &cobra.Command{
		Use:   "co [a|all]",
		Short: "Pick a GitHub PR with fzf and check it out",
		Long: `co lists open PRs from the current repo via 'gh pr list', pipes them
through fzf, and runs 'gh pr checkout' on the selection. By default only
PRs you authored are listed; pass 'a' or 'all' to see everyone's.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ghArgs := []string{"pr", "list"}
			if len(args) == 0 || (args[0] != "a" && args[0] != "all") {
				ghArgs = append(ghArgs, "--author", "@me")
			}
			list, err := runOut("gh", ghArgs...)
			if err != nil {
				return err
			}
			if list == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "no PRs to choose from")
				return nil
			}
			selected, err := fzfPick(list)
			if err != nil {
				if errors.Is(err, errCanceled) {
					return nil
				}
				return err
			}
			pr := firstField(selected)
			if pr == "" {
				return fmt.Errorf("could not parse PR number from selection: %q", selected)
			}
			return run("gh", "pr", "checkout", pr)
		},
	}
	registry.Register("co", nil, cmd)
}

func registerWip() {
	var repo string

	cmd := &cobra.Command{
		Use:   "wip [PR]",
		Short: "Browse your in-flight PRs (default repo: " + defaultPRRepo + ")",
		Long: `wip lists your open PRs in --repo and lets you fzf-pick one to view in
the browser. With a PR number argument, it prints comments + checks for
that PR (used as the fzf preview hook in interactive mode).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if err := run("gh", "pr", "view", "--comments", "--repo", repo, args[0]); err != nil {
					return err
				}
				return run("gh", "pr", "checks", "--repo", repo, args[0])
			}
			list, err := runOut("gh", "pr", "list", "--author", "@me", "--repo", repo)
			if err != nil {
				return err
			}
			if list == "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "no PRs to choose from")
				return nil
			}
			// Use a self-recursive preview so each fzf row shows its PR's
			// comments + checks. self-as-preview means we re-enter with the
			// PR number; argv[0] is preserved by the symlink farm.
			preview := fmt.Sprintf("wip --repo %s {}", repo)
			selected, err := fzfPick(list, "--preview", preview)
			if err != nil {
				if errors.Is(err, errCanceled) {
					return nil
				}
				return err
			}
			pr := firstField(selected)
			if pr == "" {
				return fmt.Errorf("could not parse PR number from selection: %q", selected)
			}
			return run("gh", "pr", "view", "--web", "--repo", repo, pr)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", defaultPRRepo, "GitHub repo to list PRs in")
	registry.Register("wip", nil, cmd)
}

func registerGhPRPreview() {
	var repo string

	cmd := &cobra.Command{
		Use:   "gh-pr-preview <PR-LINE>",
		Short: "Print PR view for an fzf preview pane (parses PR number from first column)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr := firstField(args[0])
			if pr == "" {
				return fmt.Errorf("could not parse PR number from %q", args[0])
			}
			return run("gh", "pr", "view", "--repo", repo, pr)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", defaultPRRepo, "GitHub repo to look up the PR in")
	registry.Register("gh-pr-preview", nil, cmd)
}
