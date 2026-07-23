package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui"
	"github.com/AnatolyRugalev/kube-commander/internal/version"
)

// runOptions carries the resolved root-command flags into runTUI, so the launch
// path is one testable function independent of cobra.
type runOptions struct {
	kubeconfig   string // --kubeconfig: explicit kubeconfig path ("" = standard rules)
	context      string // --context: kubeconfig context name ("" = current-context)
	namespace    string // -n/--namespace: initial watch scope ("" = all namespaces)
	namespaceSet bool   // whether -n was passed explicitly (vs. its "" default)
	configPath   string // --config: kubecom config file ("" = user config dir)
}

// runTUI is the default action of bare `kubecom`: it resolves the keymap from the
// user config, builds a live cluster client from the kubeconfig/context flags, and
// runs the Bubble Tea browse UI against it. It fails gracefully (D46) — a bad or
// missing kubeconfig/context returns a clear error and never panics — and sets up
// file logging before entering the alt-screen so nothing corrupts the terminal
// (stack.md logging rule).
func runTUI(opts runOptions) error {
	// Logging first, so a client-go/klog warning during startup lands in the file,
	// not on the terminal we are about to take over.
	logFile, err := setupLogging()
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	// Resolve the effective keymap (defaults + config overrides), mirroring
	// `kubecom keys`. Merge warnings go to the log, not stderr (the TUI owns it).
	path := opts.configPath
	if path == "" {
		p, err := config.Path()
		if err != nil {
			return err
		}
		path = p
	}

	// One-shot legacy-config migration (M2-12b): before loading the config, if no
	// new-format config exists yet at path and a legacy ~/.kubecom.yaml is present,
	// migrate it — write a fresh config once (a present config then suppresses
	// re-migration) and carry the "what to redo by hand" notes into a startup toast.
	// Degrades to no migration and never blocks launch (principle 3, D92).
	startupErr := maybeMigrate(path)

	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	km, warnings, err := cfg.Keymap()
	if err != nil {
		return err
	}
	for _, w := range warnings {
		slog.Warn("keymap merge", "warning", w)
	}

	// Build the live cluster client. Connect resolves the rest.Config (bad/missing
	// kubeconfig or context surfaces here as KindBadContext) and wires the client-go
	// handles without a network round-trip; connection faults appear later, in the UI.
	clients, err := kube.Connect(kube.ClientConfig{
		Kubeconfig: opts.kubeconfig,
		Context:    opts.context,
	})
	if err != nil {
		if kube.Classify(err) == kube.KindBadContext {
			return fmt.Errorf("cannot start: %w (check your kubeconfig and --context)", err)
		}
		return fmt.Errorf("cannot start: %w", err)
	}

	// Resolve the active context once — it names both the status bar and the
	// per-context menu file. Load that file's extra resource entries (FB-menu-config-03,
	// D83): a missing file yields no extras (the default menu), while a malformed one
	// degrades to the default menu and surfaces a transient startup toast rather than
	// blocking launch (principle 3). A warning also lands in the log.
	ctxName := kube.ContextName(kube.ClientConfig{
		Kubeconfig: opts.kubeconfig,
		Context:    opts.context,
	})
	menuExtras, menuErr := loadMenuExtras(ctxName)
	if menuErr != nil {
		slog.Warn("menu config", "context", ctxName, "error", menuErr)
		// The shell surfaces a single startup toast; a first-start migration report
		// (rare, and the more notable event) wins that slot. The menu error is always
		// logged above, so it is never lost even when it does not take the toast.
		if startupErr == nil {
			e := tui.NewErrorMsg("menu config", menuErr)
			startupErr = &e
		}
	}

	// Resolve the initial watch scope and the namespace-persistence seam from the
	// per-context state store (D90/M2-11b-2). An explicit -n wins for this run and
	// does not touch the stored state (D91); with no -n, restore the last namespace
	// kubecom recorded for this context. A picked namespace is written back through
	// the persister (statePersister), so the next launch reopens on it.
	state, statePath := loadState(ctxName)
	namespace := initialNamespace(opts, state)
	var persister tui.NamespacePersister
	if statePath != "" {
		persister = &statePersister{path: statePath, state: state}
	}

	// Construct the shell over the resolved keymap with the live client wired in for
	// watches and discovery, scoped to the requested namespace. The model requests
	// the alternate screen itself (via View.AltScreen — D70), so no program option
	// is needed here.
	model := tui.NewWithKeymap(km,
		tui.WithWatcher(clients),
		tui.WithDiscoverer(clients),
		tui.WithNamespaceLister(clients),
		tui.WithYAMLGetter(clients),
		tui.WithDescriber(clients),
		tui.WithLogStreamer(clients),
		tui.WithContainerLister(clients),
		tui.WithNamespace(namespace),
		tui.WithNamespacePersister(persister),
		tui.WithContext(ctxName),
		tui.WithVersion(version.Version),
		tui.WithMenuExtras(menuExtras),
		tui.WithStartupError(startupErr),
	)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return fmt.Errorf("kubecom exited with error: %w", err)
	}
	return nil
}

// loadMenuExtras resolves the per-context menu file for the active context and
// returns its extra resource entries (D83). It degrades rather than blocks launch
// (principle 3): an unresolved context (blank name) or a missing file yields no
// extras and no error (the built-in default menu); only a malformed/unreadable
// file returns an error, which the caller turns into a startup toast + log warning
// while still launching on the default menu.
func loadMenuExtras(context string) ([]config.MenuResource, error) {
	if strings.TrimSpace(context) == "" {
		return nil, nil // no resolved context → no per-context menu file.
	}
	path, err := config.MenuPath(context)
	if err != nil {
		return nil, err
	}
	mc, err := config.LoadMenuFile(path) // a missing file returns the zero config, no error.
	if err != nil {
		return nil, err
	}
	return mc.Resources, nil
}

// maybeMigrate runs the one-shot legacy-config migration on first start (M2-12b).
// When no new-format config exists yet at configPath and a legacy ~/.kubecom.yaml
// is present, it parses that file (config.Migrate), writes a fresh new-format
// config once, and returns the migration notes as a transient startup toast so the
// user learns what could not be carried over automatically (D92). A present config
// makes migration a no-op, so it runs at most once per config file (one-shot).
//
// It degrades rather than blocks (principle 3): a present config, an absent legacy
// file, or a malformed/unreadable legacy file all yield no migration and no toast,
// logging a warning where a fault was swallowed. It never returns an error —
// migration must never keep kubecom from launching. A malformed/unreadable legacy
// file writes no config (so a later start migrates a fixed file); a failed write of
// the new config likewise leaves migration to re-run next start.
func maybeMigrate(configPath string) *tui.ErrorMsg {
	// A present new-format config means migration already ran (or the user authored
	// one): never overwrite it. os.Stat rather than config.LoadFile, which maps a
	// missing file to the zero config and so hides the present/absent distinction
	// this one-shot turns on.
	if _, err := os.Stat(configPath); err == nil {
		return nil // config exists → one-shot already satisfied.
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Warn("migration: stat config", "path", configPath, "error", err)
		return nil // cannot tell if it exists → do not risk clobbering it.
	}

	legacyPath, err := config.LegacyPath()
	if err != nil {
		slog.Warn("migration: locating legacy config", "error", err)
		return nil
	}
	f, err := os.Open(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil // no legacy file to migrate from — the common case.
	}
	if err != nil {
		slog.Warn("migration: opening legacy config", "path", legacyPath, "error", err)
		return nil
	}
	defer func() { _ = f.Close() }()

	cfg, notes, err := config.Migrate(f)
	if err != nil {
		// Unparseable legacy YAML: no migration and no config written, so a later
		// start migrates a fixed file. Never blocks (D92).
		slog.Warn("migration: parsing legacy config", "path", legacyPath, "error", err)
		return nil
	}
	if err := cfg.SaveFile(configPath); err != nil {
		// Could not persist the migrated config; skip the toast and let migration
		// re-run on the next start rather than reporting a migration that did not stick.
		slog.Warn("migration: writing new config", "path", configPath, "error", err)
		return nil
	}
	slog.Info("migrated legacy config", "from", legacyPath, "to", configPath, "notes", len(notes))
	if len(notes) == 0 {
		return nil // migrated (e.g. an empty legacy file) with nothing to report.
	}
	e := tui.NewErrorMsg("migrated ~/.kubecom.yaml", errors.New(strings.Join(notes, " ")))
	return &e
}

// initialNamespace resolves the initial watch scope from the flags and the stored
// per-context state (M2-11b-2/D91): an explicit -n wins for this run — including an
// explicit `-n ""` meaning all namespaces — overriding whatever namespace kubecom
// last recorded for the context; with no -n given, the stored last namespace is
// restored (empty when none was ever saved).
func initialNamespace(opts runOptions, state *config.State) string {
	if opts.namespaceSet {
		return opts.namespace
	}
	return state.LastNamespace
}

// loadState resolves the active context's per-context state file (D90) and the path
// a picked namespace is persisted back to. It degrades rather than blocks
// (principle 3): a blank/unresolved context yields the zero State and no path
// (persistence disabled — kubecom cannot key state without a context); a missing
// file yields the zero State (a context kubecom has never recorded state for starts
// on defaults). A malformed/unreadable state file also degrades to the zero State,
// logging a warning rather than surfacing a toast or failing launch — the state
// file is kubecom-owned (kubecom writes it), so a corrupt one is rare and the next
// namespace switch overwrites it cleanly.
func loadState(context string) (state *config.State, path string) {
	if strings.TrimSpace(context) == "" {
		return &config.State{}, ""
	}
	p, err := config.StatePath(context)
	if err != nil {
		slog.Warn("state file", "context", context, "error", err)
		return &config.State{}, ""
	}
	st, err := config.LoadStateFile(p)
	if err != nil {
		slog.Warn("state file", "context", context, "error", err)
		return &config.State{}, p
	}
	return st, p
}

// statePersister is the launcher's namespace-persistence seam (tui.NamespacePersister):
// it records a picked namespace to the active context's state file (D90/M2-11b-2).
// It is bound to one context's resolved state path at construction, so the tui
// package stays context-agnostic. The loaded State is retained and mutated in place
// so future per-context fields (should State grow any) round-trip unchanged.
type statePersister struct {
	path  string
	state *config.State
}

// PersistNamespace writes the given namespace as the context's last namespace,
// atomically replacing the state file (config.State.SaveFile → atomicWriteFile,
// 0o600). Called off the update loop by the shell whenever the picked scope changes.
func (p *statePersister) PersistNamespace(ns string) error {
	p.state.LastNamespace = ns
	return p.state.SaveFile(p.path)
}
