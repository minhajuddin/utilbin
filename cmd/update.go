package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
	"github.com/minhajuddin/utilbin/internal/updater"
)

func init() {
	var checkOnly bool
	var force bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and install the latest release of utilbin",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			msg, err := updater.CheckAndApply(ctx, Version, force, checkOnly)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), msg)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "report if a new version exists, don't install")
	cmd.Flags().BoolVar(&force, "force", false, "install latest even if it matches the running version")
	registry.RegisterMeta("update", cmd)
}
