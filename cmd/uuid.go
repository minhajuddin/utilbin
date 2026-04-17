package cmd

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	var version string
	var count int

	cmd := &cobra.Command{
		Use:   "uuid",
		Short: "Generate UUID(s) — default v4 (random)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if count < 1 {
				return fmt.Errorf("--count must be >= 1")
			}
			gen, err := uuidGenerator(version)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for i := 0; i < count; i++ {
				u, err := gen()
				if err != nil {
					return err
				}
				fmt.Fprintln(out, u.String())
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&version, "version", "v", "4", "UUID version: 4 (random) or 7 (time-ordered)")
	cmd.Flags().IntVarP(&count, "count", "n", 1, "how many UUIDs to print")
	registry.Register("uuid", nil, cmd)
}

func uuidGenerator(version string) (func() (uuid.UUID, error), error) {
	switch version {
	case "4", "v4":
		return func() (uuid.UUID, error) { return uuid.NewRandom() }, nil
	case "7", "v7":
		return uuid.NewV7, nil
	}
	if n, err := strconv.Atoi(version); err == nil {
		return nil, fmt.Errorf("unsupported UUID version: %d (try 4 or 7)", n)
	}
	return nil, fmt.Errorf("unknown UUID version: %q (try 4 or 7)", version)
}
