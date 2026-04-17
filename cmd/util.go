package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	registerLogID()
	registerRandomChars()
	registerCanIEat()
}

func registerLogID() {
	cmd := &cobra.Command{
		Use:   "log-id",
		Short: "Random hex log ID with the '6d6b' prefix (12 chars total)",
		RunE: func(cmd *cobra.Command, args []string) error {
			b := make([]byte, 4)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "6d6b%s\n", hex.EncodeToString(b))
			return nil
		},
	}
	registry.Register("log-id", nil, cmd)
}

func registerRandomChars() {
	// Compatibility shim for ~/r/dot3/bin/random-chars: takes (length, encoding)
	// where encoding is one of openssl's: base64, hex. Defaults match the
	// original (32 / base64).
	cmd := &cobra.Command{
		Use:   "random-chars [LENGTH] [ENCODING]",
		Short: "N random bytes encoded as base64 or hex (compat shim for the old script)",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			length := 32
			encoding := "base64"
			if len(args) >= 1 {
				v, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("LENGTH must be an integer: %w", err)
				}
				length = v
			}
			if len(args) >= 2 {
				encoding = args[1]
			}
			b := make([]byte, length)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch encoding {
			case "base64":
				fmt.Fprintln(out, base64.StdEncoding.EncodeToString(b))
			case "hex":
				fmt.Fprintln(out, hex.EncodeToString(b))
			default:
				return fmt.Errorf("unsupported encoding: %s (try base64 or hex)", encoding)
			}
			return nil
		},
	}
	registry.Register("random-chars", nil, cmd)
}

func registerCanIEat() {
	// Intermittent-fasting check: 16-8 window. Eat allowed 08:00–15:59.
	cmd := &cobra.Command{
		Use:   "can-i-eat",
		Short: "Intermittent fasting check — yes/no based on hour of day (08-16 window)",
		RunE: func(cmd *cobra.Command, args []string) error {
			h := time.Now().Hour()
			if h > 8 && h < 16 {
				fmt.Fprintln(cmd.OutOrStdout(), "CIE? Yes")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "CIE? No")
			}
			return nil
		},
	}
	registry.Register("can-i-eat", nil, cmd)
}
