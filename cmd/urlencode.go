package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/minhajuddin/utilbin/internal/registry"
)

func init() {
	encode := &cobra.Command{
		Use:   "urlencode [INPUT...]",
		Short: "Percent-encode INPUT (or stdin) for use in a URL",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := readInput(args)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), url.QueryEscape(s))
			return nil
		},
	}
	registry.Register("urlencode", nil, encode)

	decode := &cobra.Command{
		Use:   "urldecode [INPUT...]",
		Short: "Decode a percent-encoded INPUT (or stdin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := readInput(args)
			if err != nil {
				return err
			}
			out, err := url.QueryUnescape(s)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	registry.Register("urldecode", nil, decode)
}
