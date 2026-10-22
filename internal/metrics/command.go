// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package metrics

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tbhb-dev/devtools/internal/buildmeta"
)

var (
	errName = errors.New("--name must not be empty")
	errAttr = errors.New("--attr must be key=value with a non-empty key")
)

// Run executes metrics and returns its process exit status.
func Run(args []string, out, stderr io.Writer) int {
	root := &cobra.Command{
		Use:           "metrics",
		Short:         "Record events in a local journal",
		Version:       buildmeta.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	var config, name, itemRef string
	var attrs []string
	root.PersistentFlags().StringVar(&config, "config", "", "TOML configuration file")
	event := &cobra.Command{
		Use:   "event",
		Short: "Append an event without network access",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if name == "" {
				return errName
			}
			values := make(map[string]string, len(attrs))
			for _, attr := range attrs {
				key, value, ok := strings.Cut(attr, "=")
				if !ok || key == "" {
					return errAttr
				}
				values[key] = value
			}
			dir, err := journalDir(config)
			if err != nil {
				return err
			}
			return appendEvent(dir, name, itemRef, values)
		},
	}
	event.Flags().StringVar(&name, "name", "", "Event name (free-form dotted string)")
	event.Flags().StringVar(&itemRef, "item-ref", "", "Optional item reference")
	event.Flags().StringArrayVar(&attrs, "attr", nil, "String key=value attribute (repeatable; last value wins)")
	root.AddCommand(event)
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
