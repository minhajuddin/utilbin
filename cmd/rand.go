package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	root := &cobra.Command{
		Use:   "rand",
		Short: "Generate random values from the OS CSPRNG",
	}

	hexCmd := &cobra.Command{
		Use:   "hex [N]",
		Short: "N random bytes as hex (default 32)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := randBytes(args, 32)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), hex.EncodeToString(b))
			return nil
		},
	}
	root.AddCommand(hexCmd)

	b64Cmd := &cobra.Command{
		Use:   "base64 [N]",
		Short: "N random bytes as base64 (default 32)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := randBytes(args, 32)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), base64.StdEncoding.EncodeToString(b))
			return nil
		},
	}
	root.AddCommand(b64Cmd)

	bytesCmd := &cobra.Command{
		Use:   "bytes [N]",
		Short: "N raw random bytes (default 32) — write to stdout",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := randBytes(args, 32)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
	root.AddCommand(bytesCmd)

	intCmd := &cobra.Command{
		Use:   "int [MIN] MAX",
		Short: "Random integer in [MIN, MAX] (MIN defaults to 0)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			lo, hi, err := parseIntRange(args)
			if err != nil {
				return err
			}
			if hi < lo {
				return fmt.Errorf("max (%d) must be >= min (%d)", hi, lo)
			}
			delta := big.NewInt(hi - lo + 1)
			n, err := rand.Int(rand.Reader, delta)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), n.Int64()+lo)
			return nil
		},
	}
	root.AddCommand(intCmd)

	registry.Register("rand", nil, root)

	// Symlink-only shims so muscle memory still works at the shell — these
	// reach the same cobra commands but don't pollute `utilbin --help`.
	registry.RegisterShim("randhex", hexCmd)
	registry.RegisterShim("randbase64", b64Cmd)
	registry.RegisterShim("randint", intCmd)
	registry.RegisterShim("randbytes", bytesCmd)
}

func randBytes(args []string, def int) ([]byte, error) {
	n := def
	if len(args) > 0 {
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, err
		}
		if v < 0 {
			return nil, fmt.Errorf("N must be >= 0")
		}
		n = v
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func parseIntRange(args []string) (int64, int64, error) {
	if len(args) == 1 {
		hi, err := strconv.ParseInt(args[0], 10, 64)
		return 0, hi, err
	}
	lo, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	hi, err := strconv.ParseInt(args[1], 10, 64)
	return lo, hi, err
}
