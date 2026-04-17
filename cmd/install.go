package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	var linkDir string
	var uninstall bool
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Create symlinks for every sub-command (or remove with --uninstall)",
		Long: `install creates one symlink per registered sub-command in --link-dir,
each pointing at the running utilbin binary. This is what makes commands
like 'tm' or 'urlencode' invokable as standalone names.

Run install once after the binary is in place, then again whenever a new
release adds new command names.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if linkDir == "" {
				d, err := defaultLinkDir()
				if err != nil {
					return err
				}
				linkDir = d
			}
			self, err := selfPath()
			if err != nil {
				return err
			}
			if !dryRun {
				if err := os.MkdirAll(linkDir, 0o755); err != nil {
					return err
				}
			}

			names := registry.Names()
			// Always include "utilbin" itself so a single install can point
			// PATH at link-dir without a separate copy step.
			names = append([]string{"utilbin"}, names...)

			for _, name := range names {
				target := filepath.Join(linkDir, name)
				if uninstall {
					if err := unlinkIfOurs(target, self, force, dryRun, out); err != nil {
						return err
					}
					continue
				}
				if err := linkOne(target, self, force, dryRun, out); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&linkDir, "link-dir", "", "where to put symlinks (default ~/.local/bin)")
	cmd.Flags().BoolVar(&uninstall, "uninstall", false, "remove symlinks instead of creating them")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite/remove non-symlink entries too (dangerous)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print actions without touching the filesystem")
	registry.Register("install", nil, cmd)
}

func defaultLinkDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func selfPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

func linkOne(target, self string, force, dryRun bool, out interface{ Write([]byte) (int, error) }) error {
	info, err := os.Lstat(target)
	if err == nil {
		// Existing entry: only replace if it's already pointing at us, or
		// the user asked us to clobber.
		if info.Mode()&os.ModeSymlink != 0 {
			cur, _ := os.Readlink(target)
			if cur == self {
				fmt.Fprintf(out, "skip (current): %s\n", target)
				return nil
			}
			if !force {
				fmt.Fprintf(out, "replace symlink: %s -> %s\n", target, self)
			}
		} else if !force {
			fmt.Fprintf(out, "skip (not a symlink, use --force): %s\n", target)
			return nil
		}
		if !dryRun {
			if err := os.Remove(target); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if dryRun {
		fmt.Fprintf(out, "would link: %s -> %s\n", target, self)
		return nil
	}
	if err := os.Symlink(self, target); err != nil {
		return err
	}
	fmt.Fprintf(out, "linked: %s -> %s\n", target, self)
	return nil
}

func unlinkIfOurs(target, self string, force, dryRun bool, out interface{ Write([]byte) (int, error) }) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if !force {
			fmt.Fprintf(out, "skip (not a symlink): %s\n", target)
			return nil
		}
	} else {
		cur, _ := os.Readlink(target)
		if cur != self && !force {
			fmt.Fprintf(out, "skip (not ours): %s -> %s\n", target, cur)
			return nil
		}
	}
	if dryRun {
		fmt.Fprintf(out, "would remove: %s\n", target)
		return nil
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	fmt.Fprintf(out, "removed: %s\n", target)
	return nil
}
