package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/minhajuddin/utilbin/cmd"
	"github.com/minhajuddin/utilbin/internal/registry"
	"github.com/minhajuddin/utilbin/internal/updater"
)

// version is set at build time via -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	cmd.Version = version
	cmd.AttachAll()

	// Kick off an opportunistic background update check. Returns immediately;
	// the goroutine is best-effort and never blocks the user-facing command.
	updater.MaybeBackgroundCheck(version)

	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if base != "utilbin" && base != "" {
		if sub := registry.Find(base); sub != nil {
			if err := registry.Dispatch(sub, os.Args[1:]); err != nil {
				os.Exit(1)
			}
			return
		}
	}

	if err := cmd.Root.Execute(); err != nil {
		os.Exit(1)
	}
}
