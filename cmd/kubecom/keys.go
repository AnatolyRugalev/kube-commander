package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/keylog"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
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
	cmd.AddCommand(newKeysAnalyzeCmd())
	return cmd
}

// newKeysAnalyzeCmd builds `kubecom keys analyze <trace.jsonl>`: it reads a
// --keylog trace and turns it into the four things a UX pass asks (STORY-03,
// D268 pt 2) — the unresolved presses ranked by frequency (the dead ends), the
// action counts, the longest pauses, and the multi-key sequences the walker
// started but never finished. Sequence completion is judged against the resolved
// keymap, so a chord the config rebinds is judged by the config's meaning.
func newKeysAnalyzeCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "analyze <trace.jsonl>",
		Short: "Summarize a keystroke trace",
		Long: `analyze reads a --keylog trace and prints the four things a UX pass asks:
the presses that resolved to no action (ranked by frequency), what actions ran,
the longest pauses between presses, and the multi-key sequences that were started
but never finished. Run it on a trace walked against the story cluster:
    ./stories/cluster/up.sh
    KUBECOM_KEYLOG=~/traces/s02.jsonl kubecom
    kubecom keys analyze ~/traces/s02.jsonl`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			km, _, err := cfg.Keymap()
			if err != nil {
				return err
			}
			records, err := keylog.Read(args[0])
			if err != nil {
				return err
			}
			rep := keylog.Analyze(records, kmResolver{km}, keymap.SequenceTimeout)
			return printAnalyze(cmd.OutOrStdout(), args[0], rep)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "",
		"config file to resolve sequences against (default: user config dir /kubecom/config.yaml)")
	return cmd
}

// printAnalyze renders an analysis. Kept separate from the command so it is
// testable without cobra, like printKeys.
func printAnalyze(out io.Writer, path string, rep keylog.Report) error {
	if _, err := fmt.Fprintf(out, "%s: %d presses over %s\n", path, rep.Records, human(rep.Elapsed)); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "\nunresolved presses — reached for a key kubecom does not have"); err != nil {
		return err
	}
	if len(rep.DeadEnds) == 0 {
		if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
			return err
		}
	}
	for _, d := range rep.DeadEnds {
		if _, err := fmt.Fprintf(out, "  %-8s %2d  %s\n", d.Key, d.Count, d.Mode); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out, "\nactions"); err != nil {
		return err
	}
	if len(rep.Actions) == 0 {
		if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
			return err
		}
	}
	for _, a := range rep.Actions {
		if _, err := fmt.Fprintf(out, "  %-22s %2d\n", a.Action, a.Count); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out, "\nlongest pauses — where the walker stopped to think"); err != nil {
		return err
	}
	if len(rep.Pauses) == 0 {
		if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
			return err
		}
	}
	for _, p := range rep.Pauses {
		if _, err := fmt.Fprintf(out, "  %-8s before %-6s (%s)\n", human(p.Gap), p.NextKey, p.NextMode); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out, "\nabandoned sequences — started, never finished"); err != nil {
		return err
	}
	if len(rep.Sequences) == 0 {
		if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
			return err
		}
	}
	for _, s := range rep.Sequences {
		keys := strings.Join(s.Keys, " ")
		where := "end of trace"
		if s.AfterKey != "" {
			where = fmt.Sprintf("then %s (%s)", s.AfterKey, s.AfterMode)
		}
		if _, err := fmt.Fprintf(out, "  %-6s %s\n", keys, where); err != nil {
			return err
		}
	}
	return nil
}

// human renders a duration compactly: 5s, 1m 30s.
func human(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	return fmt.Sprintf("%dm %s", int(d.Minutes()), (d % time.Minute).Round(time.Second))
}

// kmResolver adapts the keymap's Action-returning Resolve to the keylog
// package's string-returning Resolver interface.
type kmResolver struct{ km *keymap.Keymap }

func (r kmResolver) Resolve(tokens ...string) (string, bool) {
	a, ok := r.km.Resolve(tokens...)
	return string(a), ok
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
