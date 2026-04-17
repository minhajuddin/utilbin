// Package registry collects every sub-command the binary knows about so that
// (a) the BusyBox-style argv[0] dispatcher can look one up by name and
// (b) the install command knows which symlinks to create.
package registry

import "github.com/spf13/cobra"

type Entry struct {
	Name       string
	Aliases    []string
	Cmd        *cobra.Command
	Standalone bool // attach as a top-level subcommand of Root?
	NoSymlink  bool // skip symlink creation (meta commands like install/update)
}

var entries []Entry

// Register adds a primary sub-command. It becomes both a top-level subcommand
// on Root (so `utilbin <name>` works) and a symlink target under that name
// plus any aliases. Aliases are also wired into cobra so `utilbin <alias>`
// works the same as `utilbin <name>`.
func Register(name string, aliases []string, cmd *cobra.Command) {
	if len(aliases) > 0 {
		cmd.Aliases = append(cmd.Aliases, aliases...)
	}
	entries = append(entries, Entry{Name: name, Aliases: aliases, Cmd: cmd, Standalone: true})
}

// RegisterShim adds a symlink-only entry pointing at an existing cobra
// command, typically a subcommand reachable via a longer path. Used so that
// `randhex` works as a standalone command but doesn't pollute `utilbin --help`
// (which already shows it as `rand hex`).
func RegisterShim(name string, cmd *cobra.Command) {
	entries = append(entries, Entry{Name: name, Cmd: cmd, Standalone: false})
}

// RegisterMeta adds a top-level subcommand that is reachable only as
// `utilbin <name>`. No symlink is created, so common names like `install` and
// `update` don't shadow system binaries on PATH.
func RegisterMeta(name string, cmd *cobra.Command) {
	entries = append(entries, Entry{Name: name, Cmd: cmd, Standalone: true, NoSymlink: true})
}

func All() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// Names returns every registered name plus every alias, omitting NoSymlink
// entries. Used by `utilbin install` to decide what symlinks to create.
func Names() []string {
	var names []string
	for _, e := range entries {
		if e.NoSymlink {
			continue
		}
		names = append(names, e.Name)
		names = append(names, e.Aliases...)
	}
	return names
}

func Find(name string) *cobra.Command {
	for _, e := range entries {
		if e.Name == name {
			return e.Cmd
		}
		for _, a := range e.Aliases {
			if a == name {
				return e.Cmd
			}
		}
	}
	return nil
}

// Dispatch runs cmd as if it were invoked at the top level, even if it is a
// nested subcommand. Cobra's Execute always walks up to the root, so we
// reconstruct the path (e.g. ["rand", "hex"] for the `randhex` shim) and feed
// it to the root command. extraArgs are user-supplied arguments that come
// after the command path.
func Dispatch(cmd *cobra.Command, extraArgs []string) error {
	var path []string
	for cur := cmd; cur != nil && cur.HasParent(); cur = cur.Parent() {
		path = append([]string{cur.Name()}, path...)
	}
	root := cmd.Root()
	root.SetArgs(append(path, extraArgs...))
	return root.Execute()
}
