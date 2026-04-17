package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

// Shell function shims live here. Most utilbin commands work as plain
// binaries; a handful (e.g. anything that needs to mutate the parent shell's
// CWD) require a shell function that calls utilbin and acts on its output.
//
// To use, eval the output in your rc file:
//   eval "$(utilbin shell-init zsh)"

type shimSet struct{ posix, fish string }

// posixShims/fishShims are populated by individual cmd/*.go files via
// registerShim. Empty for now — wt/tm-style helpers will register theirs
// when ported.
var registeredShims []shimSet

// registerShellShim is called from init() in commands that need a wrapper.
// posix is sourced into bash/zsh; fish is sourced into fish.
func registerShellShim(posix, fish string) {
	registeredShims = append(registeredShims, shimSet{posix: posix, fish: fish})
}

func init() {
	cmd := &cobra.Command{
		Use:       "shell-init [zsh|bash|fish]",
		Short:     "Print shell function shims for `eval` in your rc file",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"zsh", "bash", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			shell := args[0]

			fmt.Fprintf(out, "# utilbin shell-init for %s\n", shell)
			switch shell {
			case "zsh", "bash":
				for _, s := range registeredShims {
					if s.posix == "" {
						continue
					}
					fmt.Fprintln(out, strings.TrimRight(s.posix, "\n"))
				}
			case "fish":
				for _, s := range registeredShims {
					if s.fish == "" {
						continue
					}
					fmt.Fprintln(out, strings.TrimRight(s.fish, "\n"))
				}
			default:
				return fmt.Errorf("unsupported shell: %s (try zsh, bash, fish)", shell)
			}
			return nil
		},
	}
	registry.Register("shell-init", nil, cmd)
}
