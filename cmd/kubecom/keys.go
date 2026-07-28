package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
	"github.com/spf13/cobra"
)

// newKeysCmd builds `kubecom keys`: it prints the effective keymap — every
// action and the keys bound to it after merging the config's keys: section onto
// the vim-first defaults — so a user can see exactly what their config resolves
// to (D11: the keymap is the single source of truth for keys). Merge warnings go
// to stderr; a genuinely invalid keymap (unknown action / bad token / collision)
// fails the command.
func newKeysCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Print the resolved keybindings",
		Long: `keys prints the effective keymap: every action and the keys bound to it
after merging your config's keys: section onto the vim-first defaults, plus any
warnings from that merge. Disabled actions are shown as "(disabled)".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := configPath
			if path == "" {
				p, err := config.Path()
				if err != nil {
					return err
				}
				path = p
			}
			cfg, err := config.LoadFile(path)
			if err != nil {
				return err
			}
			km, warnings, err := cfg.Keymap()
			if err != nil {
				return err
			}
			return printKeys(cmd.OutOrStdout(), cmd.ErrOrStderr(), km, warnings)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "",
		"config file to resolve (default: user config dir /kubecom/config.yaml)")
	return cmd
}

// printKeys renders the resolved keymap in registry order, warnings first (to
// errOut). Kept separate from the command so it is testable without cobra.
//
// The two left columns are sized from the widest value actually printed rather than
// from a fixed constant: registering one long action id (search.allNamespaces was the
// first to pass 18 columns) must not silently push every following row's description
// out of alignment.
func printKeys(out, errOut io.Writer, km *keymap.Keymap, warnings []string) error {
	for _, warn := range warnings {
		if _, err := fmt.Fprintln(errOut, "warning:", warn); err != nil {
			return err
		}
	}
	actions := keymap.Actions()
	rows := make([][2]string, 0, len(actions))
	idWidth, keyWidth := 0, 0
	for _, a := range actions {
		keys := strings.Join(km.Keys(a), ", ")
		if keys == "" {
			keys = "(disabled)"
		}
		rows = append(rows, [2]string{string(a), keys})
		idWidth = max(idWidth, len(string(a)))
		keyWidth = max(keyWidth, len(keys))
	}
	for i, a := range actions {
		if _, err := fmt.Fprintf(out, "%-*s %-*s %s\n",
			idWidth, rows[i][0], keyWidth, rows[i][1], a.Describe()); err != nil {
			return err
		}
	}
	return nil
}
