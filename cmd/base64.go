package cmd

import (
	"encoding/base64"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	var urlSafe bool
	var noPad bool

	enc := func() *base64.Encoding {
		switch {
		case urlSafe && noPad:
			return base64.RawURLEncoding
		case urlSafe:
			return base64.URLEncoding
		case noPad:
			return base64.RawStdEncoding
		default:
			return base64.StdEncoding
		}
	}

	encode := &cobra.Command{
		Use:   "base64encode [INPUT...]",
		Short: "Base64-encode INPUT (or stdin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readBinaryInput(args)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), enc().EncodeToString(data))
			return nil
		},
	}
	encode.Flags().BoolVar(&urlSafe, "url", false, "use url-safe alphabet")
	encode.Flags().BoolVar(&noPad, "no-pad", false, "omit '=' padding")
	registry.Register("base64encode", []string{"b64encode"}, encode)

	decode := &cobra.Command{
		Use:   "base64decode [INPUT...]",
		Short: "Base64-decode INPUT (or stdin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := readInput(args)
			if err != nil {
				return err
			}
			out, err := enc().DecodeString(s)
			if err != nil {
				// Try the other padding variant before giving up — easy
				// usability win when callers paste a raw value.
				if alt, altErr := tryAllBase64(s); altErr == nil {
					out = alt
				} else {
					return err
				}
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	decode.Flags().BoolVar(&urlSafe, "url", false, "use url-safe alphabet")
	decode.Flags().BoolVar(&noPad, "no-pad", false, "input has no '=' padding")
	registry.Register("base64decode", []string{"b64decode"}, decode)
}

func tryAllBase64(s string) ([]byte, error) {
	for _, e := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if out, err := e.DecodeString(s); err == nil {
			return out, nil
		}
	}
	return nil, fmt.Errorf("base64: invalid input")
}
