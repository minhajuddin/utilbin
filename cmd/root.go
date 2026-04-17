package cmd

import (
	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

// Version is overridden at build time via -ldflags "-X main.version=..."
// (main copies it onto the Root command before Execute).
var Version = "dev"

var Root = &cobra.Command{
	Use:           "utilbin",
	Short:         "Kitchen-sink CLI utilities with self-update",
	SilenceUsage:  true,
	SilenceErrors: false,
}

// AttachAll wires every registered sub-command onto Root. Called once from
// main after package init() side-effects have populated the registry.
func AttachAll() {
	Root.Version = Version
	Root.SetVersionTemplate("{{.Version}}\n")
	for _, e := range registry.All() {
		if e.Standalone {
			Root.AddCommand(e.Cmd)
		}
	}
}
