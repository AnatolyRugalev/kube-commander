package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/neuroplastio/kubecom/internal/config"
	"github.com/neuroplastio/kubecom/internal/keybind"
	"github.com/neuroplastio/kubecom/internal/keylog"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/spf13/cobra"
)

// newKeysCmd builds `kubecom keys`: it lists the keymap in force — default.kdl,
// then the `keymap` in config.kdl, then the overlay's — one line per binding with
// the context it holds in and the file:line it comes from, so a user can see
// exactly what their config resolves to (D11, D303 pt 7). A keymap kubecom would
// refuse at launch fails the command with the same file:line.
func newKeysCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "List the key bindings in force",
		Long: `keys lists the keymap in force: kubecom's default.kdl, then the keymap in
your config.kdl, then the one kubecom wrote to config.overlay.kdl. Each line is
one binding, in the order they load: its keys, the action it runs (and the value
it gives it), the context it holds in (empty for everywhere), and the file and
line it comes from. A binding in a deeper context beats one around it; at the same
depth your keymap beats the defaults, and a later line an earlier one. The actions
no key runs are listed last: the command palette (:) reaches every one.

  kubecom keys            list the bindings in force
  kubecom keys default    print default.kdl, to read or to start a keymap from
  kubecom keys check      check your keymap without starting kubecom`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			km, err := loadKeymap(configPath)
			if err != nil {
				return err
			}
			return printKeys(cmd.OutOrStdout(), km)
		},
	}
	cmd.Flags().StringVar(&configPath, "config-dir", "",
		"directory holding config.kdl (default: user config dir /kubecom)")
	cmd.AddCommand(newKeysDefaultCmd(), newKeysCheckCmd(), newKeysAnalyzeCmd())
	return cmd
}

// newKeysDefaultCmd builds `kubecom keys default`: default.kdl as kubecom embeds it.
func newKeysDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default",
		Short: "Print the default keymap (default.kdl)",
		Long: `default prints kubecom's default keymap, default.kdl, as built in. Read it to
see what each key does and where, or copy it into config.kdl's keymap under
base "none" to start a keymap of your own from it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(keymap.DefaultSource())
			return err
		},
	}
}

// newKeysCheckCmd builds `kubecom keys check`: the launch's check of the user's
// keymap, without the launch.
func newKeysCheckCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check your keymap",
		Long: `check reads the keymap in config.kdl and config.overlay.kdl the way kubecom
does at launch — every key, context and action, and the values given — and
reports each mistake at its file and line. It exits non-zero when kubecom would
refuse to start.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			km, err := loadKeymap(configPath)
			if err != nil {
				return err
			}
			user := 0
			for _, e := range km.Engine().Bindings() {
				if e.Binding.Layer() == keybind.User {
					user++
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "keymap ok: %d bindings of yours over the defaults\n", user)
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config-dir", "",
		"directory holding config.kdl (default: user config dir /kubecom)")
	return cmd
}

// loadKeymap is the keymap the launcher builds from the config directory (the
// --config-dir flag, or the default): any error is the launch's, at its file:line.
func loadKeymap(flag string) (*keymap.Keymap, error) {
	dir, err := configDir(flag)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return nil, err
	}
	docs, err := keymap.UserDocs(cfg)
	if err != nil {
		return nil, err
	}
	return keymap.New(docs...)
}

// newKeysAnalyzeCmd builds `kubecom keys analyze <trace.jsonl>`: it reads a
// --keylog trace and turns it into the five things a UX pass asks (STORY-03,
// D268 pt 2) — the unresolved presses ranked by frequency (the dead ends), the
// presses on text surfaces (reported separately since they may be ordinary
// typing or a swallowed reach, STORY-06e), the action counts, the longest
// pauses, and the multi-key sequences the walker started but never finished.
// Sequence completion is judged against the resolved
// keymap, so a chord the config rebinds is judged by the config's meaning.
func newKeysAnalyzeCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "analyze <trace.jsonl>",
		Short: "Summarize a keystroke trace",
		Long: `analyze reads a --keylog trace and prints the five things a UX pass asks:
the presses that resolved to no action (ranked by frequency), the presses on text
surfaces (which may be ordinary typing or a swallowed reach), what actions ran,
the longest pauses between presses, and the multi-key sequences that were started
but never finished. Run it on a trace walked against the story cluster:
    ./stories/cluster/up.sh
    KUBECOM_KEYLOG=~/traces/s02.jsonl kubecom
    kubecom keys analyze ~/traces/s02.jsonl`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			km, err := loadKeymap(configPath)
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
	cmd.Flags().StringVar(&configPath, "config-dir", "",
		"directory holding config.kdl to resolve sequences against (default: user config dir /kubecom)")
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

	if _, err := fmt.Fprintln(out, "\npresses on text surfaces — typed, or a reach the surface swallowed"); err != nil {
		return err
	}
	if len(rep.TextPresses) == 0 {
		if _, err := fmt.Fprintln(out, "  (none)"); err != nil {
			return err
		}
	}
	for _, t := range rep.TextPresses {
		if _, err := fmt.Fprintf(out, "  %-8s %2d  %s\n", t.Key, t.Count, t.Mode); err != nil {
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
// package's string-returning Resolver interface. A trace records which surface took
// each press, not the whole context path, so a run of keys is read where nothing is
// open: the multi-key sequences the analyser asks about are bound everywhere.
type kmResolver struct{ km *keymap.Keymap }

func (r kmResolver) Resolve(tokens ...string) (string, bool) {
	a, ok := r.km.Resolve(keybind.Path{}, tokens...)
	return string(a), ok
}

// printKeys renders the keymap's bindings in load order, then the actions no key
// runs. Kept separate from the command so it is testable without cobra. The columns
// are sized from the widest value printed, so one long action or context does not
// push every following row out of alignment.
func printKeys(out io.Writer, km *keymap.Keymap) error {
	rows := [][4]string{{"KEYS", "ACTION", "CONTEXT", "FROM"}}
	bound := map[keymap.Action]bool{}
	for _, e := range km.Engine().Bindings() {
		b := e.Binding
		action := "(unbind)"
		if !b.Unbind() {
			action = b.Action
			for _, v := range b.Args {
				action += " " + strconv.Quote(v)
			}
			bound[keymap.Action(b.Action)] = true
		}
		rows = append(rows, [4]string{keybind.FormatKeys(e.Keys), action, b.Context(), filepath.Base(b.Source())})
	}
	var widths [3]int
	for _, r := range rows {
		for i := range widths {
			widths[i] = max(widths[i], len(r[i]))
		}
	}
	for _, r := range rows {
		line := fmt.Sprintf("%-*s  %-*s  %-*s  %s", widths[0], r[0], widths[1], r[1], widths[2], r[2], r[3])
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	var unbound []string
	for _, a := range keymap.Actions() {
		if !bound[a] {
			unbound = append(unbound, string(a))
		}
	}
	if len(unbound) > 0 {
		if _, err := fmt.Fprintf(out, "\nno key (the palette runs them): %s\n", strings.Join(unbound, ", ")); err != nil {
			return err
		}
	}
	return nil
}
