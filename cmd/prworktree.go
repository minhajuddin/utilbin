package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

var rePRWorktreeURL = regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+)/pull/([0-9]+)$`)

func init() {
	root := &cobra.Command{
		Use:   "pr-worktree <PR_URL>",
		Short: "Create a sparse-checkout worktree for reviewing a GitHub PR",
		Long: `pr-worktree clones the PR's repo bare into $PR_WORKTREE_HOME/repos and
adds a sparse worktree under $PR_WORKTREE_HOME/checkouts containing only
the files changed by the PR. Cross-repo PRs (forks) are handled by
adding a fork-<owner> remote. Default $PR_WORKTREE_HOME is ~/.pr-worktrees.

Subcommands:
  pr-worktree <URL>           create sparse worktree
  pr-worktree cleanup <URL>   remove a worktree
  pr-worktree list            list active worktrees`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return prwCreate(cmd, args[0])
		},
	}

	cleanupCmd := &cobra.Command{
		Use:   "cleanup <PR_URL>",
		Short: "Remove the sparse worktree for a given PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return prwCleanup(cmd, args[0])
		},
	}
	root.AddCommand(cleanupCmd)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List active PR worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return prwList(cmd)
		},
	}
	root.AddCommand(listCmd)

	registry.Register("pr-worktree", nil, root)
}

type prwHome struct {
	root      string
	repos     string
	checkouts string
}

func prwHomeDirs() (prwHome, error) {
	base := os.Getenv("PR_WORKTREE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return prwHome{}, err
		}
		base = filepath.Join(home, ".pr-worktrees")
	}
	return prwHome{
		root:      base,
		repos:     filepath.Join(base, "repos"),
		checkouts: filepath.Join(base, "checkouts"),
	}, nil
}

type prwTarget struct {
	host, owner, repo string
	number            string
}

func parsePRURL(raw string) (prwTarget, error) {
	url := raw
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		url = url[:i]
	}
	url = strings.TrimRight(url, "/")
	m := rePRWorktreeURL.FindStringSubmatch(url)
	if m == nil {
		return prwTarget{}, fmt.Errorf("invalid PR URL %q (want https://<host>/<owner>/<repo>/pull/<number>)", raw)
	}
	return prwTarget{host: m[1], owner: m[2], repo: m[3], number: m[4]}, nil
}

type prwMeta struct {
	HeadRefName         string `json:"headRefName"`
	HeadRefOid          string `json:"headRefOid"`
	BaseRefName         string `json:"baseRefName"`
	IsCrossRepository   bool   `json:"isCrossRepository"`
	HeadRepositoryOwner struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	HeadRepository struct {
		Name string `json:"name"`
	} `json:"headRepository"`
	State string `json:"state"`
	Title string `json:"title"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

func fetchPRMeta(url string) (prwMeta, error) {
	var m prwMeta
	out, err := runOut("gh", "pr", "view", url,
		"--json", "headRefName,headRefOid,baseRefName,files,isCrossRepository,headRepositoryOwner,headRepository,state,title")
	if err != nil {
		return m, fmt.Errorf("failed to fetch PR metadata: %w", err)
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return m, fmt.Errorf("parse gh json: %w", err)
	}
	if len(m.Files) == 0 {
		return m, fmt.Errorf("PR has no changed files")
	}
	return m, nil
}

func prwCreate(cmd *cobra.Command, url string) error {
	target, err := parsePRURL(url)
	if err != nil {
		return err
	}
	dirs, err := prwHomeDirs()
	if err != nil {
		return err
	}

	meta, err := fetchPRMeta(url)
	if err != nil {
		return err
	}

	switch meta.State {
	case "MERGED":
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: PR #%s is merged; checking out head commit as it was at merge time.\n", target.number)
	case "CLOSED":
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: PR #%s is closed (not merged).\n", target.number)
	}

	bareDir := filepath.Join(dirs.repos, target.host, target.owner, target.repo+".git")
	if err := ensureBareClone(bareDir, target); err != nil {
		return err
	}
	if meta.IsCrossRepository {
		if err := ensureForkRemote(bareDir, target.host, meta.HeadRepositoryOwner.Login, meta.HeadRepository.Name); err != nil {
			return err
		}
	}
	if err := fetchPRBranch(bareDir, target.number, meta.HeadRefName, meta.IsCrossRepository, meta.HeadRepositoryOwner.Login); err != nil {
		return err
	}

	worktreePath := filepath.Join(dirs.checkouts, target.host, target.owner, target.repo, "pr-"+target.number)
	files := make([]string, 0, len(meta.Files))
	for _, f := range meta.Files {
		files = append(files, f.Path)
	}

	if err := setupWorktree(worktreePath, bareDir, meta.HeadRefOid, files); err != nil {
		// Best-effort cleanup of partial worktree.
		_ = exec.Command("git", "-C", bareDir, "worktree", "remove", "--force", worktreePath).Run()
		_ = os.RemoveAll(worktreePath)
		return err
	}

	prwPrintSummary(cmd, target, meta, worktreePath, len(files))
	return nil
}

// ensureBareClone clones the repo bare with blob filtering if it isn't
// already on disk, and sets the standard fetch refspec.
func ensureBareClone(bareDir string, t prwTarget) error {
	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		return err
	}
	repoURL := fmt.Sprintf("git@github.com:%s/%s.git", t.owner, t.repo)
	if err := run("git", "clone", "--bare", "--filter=blob:none", repoURL, bareDir); err != nil {
		return fmt.Errorf("clone %s: %w", repoURL, err)
	}
	return run("git", "-C", bareDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/heads/*")
}

func ensureForkRemote(bareDir, host, forkOwner, forkRepo string) error {
	remoteName := "fork-" + forkOwner
	if err := exec.Command("git", "-C", bareDir, "remote", "get-url", remoteName).Run(); err == nil {
		return nil
	}
	forkURL := fmt.Sprintf("https://%s/%s/%s.git", host, forkOwner, forkRepo)
	if err := run("git", "-C", bareDir, "remote", "add", remoteName, forkURL); err != nil {
		return err
	}
	return run("git", "-C", bareDir, "config", "remote."+remoteName+".fetch",
		"+refs/heads/*:refs/remotes/"+remoteName+"/*")
}

func fetchPRBranch(bareDir, prNumber, headRef string, crossRepo bool, forkOwner string) error {
	source := "origin"
	if crossRepo {
		source = "fork-" + forkOwner
	}
	if err := exec.Command("git", "-C", bareDir, "fetch", source,
		"+refs/heads/"+headRef+":refs/heads/"+headRef).Run(); err == nil {
		return nil
	}
	fmt.Fprintf(os.Stderr, "warning: could not fetch branch %q directly, falling back to refs/pull/%s/head\n", headRef, prNumber)
	return run("git", "-C", bareDir, "fetch", "origin",
		"+refs/pull/"+prNumber+"/head:refs/pr/"+prNumber+"/head")
}

func setupWorktree(worktreePath, bareDir, headSHA string, files []string) error {
	if info, err := os.Stat(worktreePath); err == nil && info.IsDir() {
		fmt.Fprintf(os.Stderr, "warning: worktree already exists at %s — updating sparse-checkout patterns.\n", worktreePath)
		if _, err := withInputPipe(strings.Join(files, "\n")+"\n",
			"git", "-C", worktreePath, "sparse-checkout", "set", "--no-cone", "--stdin"); err != nil {
			return err
		}
		return run("git", "-C", worktreePath, "checkout", headSHA, "--")
	}

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return err
	}
	if err := run("git", "-C", bareDir, "worktree", "add", "--no-checkout", "--detach", worktreePath, headSHA); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	if _, err := withInputPipe(strings.Join(files, "\n")+"\n",
		"git", "-C", worktreePath, "sparse-checkout", "set", "--no-cone", "--stdin"); err != nil {
		return err
	}
	return run("git", "-C", worktreePath, "checkout", headSHA)
}

func prwPrintSummary(cmd *cobra.Command, t prwTarget, m prwMeta, worktreePath string, fileCount int) {
	out := cmd.OutOrStdout()
	// If stdout isn't a tty, print only the path so callers can use it
	// in a pipeline (matches the original script).
	if f, ok := out.(*os.File); ok {
		if info, err := f.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
			fmt.Fprintln(out, worktreePath)
			return
		}
	}
	short := m.HeadRefOid
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "PR #%s: %s\n", t.number, m.Title)
	fmt.Fprintf(out, "Branch: %s -> %s\n", m.HeadRefName, m.BaseRefName)
	fmt.Fprintf(out, "Commit: %s\n", short)
	fmt.Fprintf(out, "Files:  %d changed\n", fileCount)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Worktree ready at:")
	fmt.Fprintln(out, "  "+worktreePath)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "To clean up:")
	fmt.Fprintf(out, "  utilbin pr-worktree cleanup https://%s/%s/%s/pull/%s\n", t.host, t.owner, t.repo, t.number)
}

func prwCleanup(cmd *cobra.Command, url string) error {
	target, err := parsePRURL(url)
	if err != nil {
		return err
	}
	dirs, err := prwHomeDirs()
	if err != nil {
		return err
	}
	bareDir := filepath.Join(dirs.repos, target.host, target.owner, target.repo+".git")
	worktreePath := filepath.Join(dirs.checkouts, target.host, target.owner, target.repo, "pr-"+target.number)
	if _, err := os.Stat(worktreePath); err != nil {
		return fmt.Errorf("no worktree found at %s", worktreePath)
	}
	if _, err := os.Stat(bareDir); err == nil {
		_ = exec.Command("git", "-C", bareDir, "worktree", "remove", "--force", worktreePath).Run()
	}
	if err := os.RemoveAll(worktreePath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed worktree for PR #%s: %s\n", target.number, worktreePath)

	// Best-effort: prune now-empty parent dirs.
	for _, p := range []string{
		filepath.Join(dirs.checkouts, target.host, target.owner, target.repo),
		filepath.Join(dirs.checkouts, target.host, target.owner),
		filepath.Join(dirs.checkouts, target.host),
	} {
		_ = os.Remove(p) // os.Remove only removes empty dirs; non-empty errors are fine.
	}
	return nil
}

func prwList(cmd *cobra.Command) error {
	dirs, err := prwHomeDirs()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dirs.checkouts); err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No active worktrees.")
		return nil
	}

	var rows [][3]string
	err = filepath.WalkDir(dirs.checkouts, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dirs.checkouts, path)
		if err != nil {
			return nil
		}
		// Layout: host/owner/repo/pr-N → 4 path segments deep.
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) != 4 || !strings.HasPrefix(parts[3], "pr-") {
			return nil
		}
		prNum := strings.TrimPrefix(parts[3], "pr-")
		repoPath := filepath.Join(parts[0], parts[1], parts[2])
		rows = append(rows, [3]string{"#" + prNum, repoPath, path})
		return nil
	})
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No active worktrees.")
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-8s  %-50s  %s\n", "PR", "REPO", "PATH")
	fmt.Fprintf(out, "%-8s  %-50s  %s\n", "------", strings.Repeat("-", 50), "----")
	for _, r := range rows {
		fmt.Fprintf(out, "%-8s  %-50s  %s\n", r[0], r[1], r[2])
	}
	return nil
}
