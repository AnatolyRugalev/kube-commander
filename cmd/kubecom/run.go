package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/styles"
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

	// Resolve the configured theme (M4-12a). An unknown name never fails the launch:
	// it degrades to the built-in default palette and reports itself, like every
	// other config fault on this path (principle 3). Last in the startup-toast
	// precedence chain — a migration report and a fallen-back menu are both larger
	// events than a mistyped palette name, which is visible on screen anyway.
	theme, themeErr := resolveTheme(cfg.Theme)
	if themeErr != nil {
		slog.Warn("theme", "configured", cfg.Theme, "error", themeErr)
		if startupErr == nil {
			e := tui.NewErrorMsg("theme", themeErr)
			startupErr = &e
		}
	}

	// Resolve the editor once, here, rather than at `e`-press time (EDIT-01/D192): it
	// is what lets the choice be logged, so a user learns which editor they will get
	// *before* they press `e` on a live object instead of discovering a broken fallback
	// at the moment they wanted to change something. A box with none of the candidates
	// installed degrades like every other startup fault — the launch proceeds, the shell
	// is edit-inert with an actionable message, and it takes the last startup-toast slot
	// (behind migration, menu and theme, all of which are larger events).
	editorArgv, editorErr := tui.ResolveEditor()
	if editorErr != nil {
		slog.Warn("editor", "error", editorErr)
		if startupErr == nil {
			e := tui.NewErrorMsg("editor", editorErr)
			startupErr = &e
		}
	} else {
		slog.Info("editor", "argv", editorArgv)
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
	// The kinds pinned on this context join the authored menu entries (CRD-PIN-01/D193):
	// one merged list goes into the menu, so a pinned CRD and a hand-written one render
	// the same row through the same menu.AddExtras path.
	menuExtras = mergeMenuExtras(menuExtras, state.PinnedResources)

	// Construct the shell over the resolved keymap with the live client wired in for
	// watches and discovery, scoped to the requested namespace. The model requests
	// the alternate screen itself (via View.AltScreen — D70), so no program option
	// is needed here. Everything cluster-bound goes in as one bundle (M4-02) so a
	// context switch can repoint it wholesale; the options beside it are per-context
	// or per-launch state, not clients.
	model := tui.NewWithKeymap(km,
		tui.WithCluster(clusterFor(clients)),
		tui.WithClusterConnector(contextConnector{kubeconfig: opts.kubeconfig}),
		tui.WithContextLister(contextLister{kubeconfig: opts.kubeconfig}),
		tui.WithContextStateLoader(contextStateLoader{}),
		tui.WithNamespace(namespace),
		tui.WithNamespacePersister(persister),
		tui.WithContext(ctxName),
		tui.WithKubeconfig(opts.kubeconfig),
		tui.WithVersion(version.Version),
		tui.WithMenuExtras(menuExtras),
		tui.WithTheme(theme),
		// The write side of the same field: a theme picked in the UI is written back
		// to this config file, so the next launch resolves it above (M4-12b-2).
		tui.WithThemePersister(&configThemePersister{path: path}),
		tui.WithStartupError(startupErr),
		// The editor resolved (and logged) above — per-process, so it is wired here
		// beside the other per-launch state rather than inside the cluster bundle.
		tui.WithEditorArgv(editorArgv),
		// The same file logger setupLogging installed as the slog default — the shell
		// records every error it toasts there, so a failure the 5s toast outlived is
		// still diagnosable afterwards (D159). Wired here rather than read as a global
		// inside the shell so a test can point it at a buffer.
		tui.WithLogger(slog.Default()),
	)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		return fmt.Errorf("kubecom exited with error: %w", err)
	}
	return nil
}

// clusterFor turns a connected client into the shell's cluster bundle (M4-02): the
// one place in the launcher where a *kube.Clients becomes the seams the TUI drives.
// The context switcher (M4-04) connects a second client and calls this same function,
// so a seam can never be wired at launch and forgotten on switch — which is precisely
// how the 21 individual With* options used to fail.
//
// *kube.Clients satisfies tui.ClusterClient directly; only the port-forward seam needs
// the PortForwarderFunc adapter, because Clients.PortForward returns the concrete
// *kube.PortForward rather than the tui.ActiveForward the shell observes.
func clusterFor(clients *kube.Clients) tui.Cluster {
	return tui.NewCluster(clients, tui.PortForwarderFunc(
		func(ctx context.Context, ref kube.ObjectRef, ports []string) (tui.ActiveForward, error) {
			return clients.PortForward(ctx, ref, ports)
		},
	))
}

// contextConnector is the launcher's tui.ClusterConnector seam (M4-04a): it connects
// to another kubeconfig context on demand and returns the shell's cluster bundle for
// it. It is bound to the `--kubeconfig` path kubecom launched with, not to a context,
// so every switch resolves against the same kubeconfig the launch client did — the
// flag names the *file*, while the context is what the switch changes.
//
// It reuses kube.Connect and clusterFor, so the switched-in cluster is wired exactly
// like the launch one: a seam added to clusterFor is live on both, and can never be
// wired at launch but forgotten on switch (D155 pt 2). A connect failure is returned
// as-is for the shell to classify and toast — the shell keeps browsing the cluster it
// is on (principle 3), so an unreachable or misconfigured context costs nothing.
type contextConnector struct {
	kubeconfig string
}

// ConnectCluster builds the cluster bundle for the named context. Like the launch
// path, kube.Connect resolves the rest.Config without a network round-trip, so a
// wrong context name or a broken kubeconfig fails here (KindBadContext) while a
// genuinely unreachable server surfaces later, in the UI, on the first watch.
func (c contextConnector) ConnectCluster(name string) (tui.Cluster, error) {
	clients, err := kube.Connect(kube.ClientConfig{
		Kubeconfig: c.kubeconfig,
		Context:    name,
	})
	if err != nil {
		return tui.Cluster{}, err
	}
	return clusterFor(clients), nil
}

// contextLister is the launcher's tui.ContextLister seam (M4-04b): it lists the
// contexts the switcher offers, straight from kube.Contexts (M4-01). Like
// contextConnector it is bound to the --kubeconfig path and *not* to a context —
// the flag names the file, the switch changes the context — so both halves of a
// switch resolve against the same kubeconfig for the whole session.
//
// It deliberately passes no Context: kube.Contexts would only use it to compute the
// Current flag, and the shell does not read that flag. kubecom's switch is
// session-scoped and never rewrites the kubeconfig's current-context, so the
// picker marks the context the *shell* is on (D158) — asking the kubeconfig would
// mark the one the reader started from.
type contextLister struct {
	kubeconfig string
}

// Contexts reads the kubeconfig and returns its declared contexts, sorted by name.
// It performs no network I/O; a missing or malformed kubeconfig comes back as a
// classified error the shell toasts (principle 3).
func (c contextLister) Contexts() ([]kube.ContextInfo, error) {
	return kube.Contexts(kube.ClientConfig{Kubeconfig: c.kubeconfig})
}

// contextStateLoader is the launcher's tui.ContextStateLoader seam (M4-05): it
// re-resolves the state that is keyed by the *kubeconfig context* — the per-context
// menu file (D83) and the per-context state file (D90) — for the context a switch
// is landing on. It is what stops a switch from carrying the previous context's
// menu additions, namespace and state-file path into the new cluster.
//
// It holds no state of its own, deliberately: everything it needs is derived from
// the context name it is handed, through the same loadMenuExtras/loadState helpers
// the launch path uses, so launch and switch can never resolve a context
// differently. Unlike contextConnector/contextLister it is not bound to
// --kubeconfig — neither file is inside the kubeconfig; both are kubecom's own,
// keyed by context name in kubecom's config/state dirs.
type contextStateLoader struct{}

// LoadContextState resolves the named context's menu extras, last-used namespace
// and namespace persister. It never fails (the interface has no error): a missing
// file is the common case and yields the default menu / all-namespaces scope, while
// a malformed or unreadable one is logged and degrades the same way rather than
// blocking the switch (principle 3). The log is the file logger setupLogging
// installed, so a degraded switch is still diagnosable afterwards (D159) — a toast
// is not available here, since the shell surfaces its own "switched to <ctx>" notice
// in the same frame.
//
// Unlike the launch path there is no -n flag to honour: the flag names the scope for
// the run kubecom was started with, not for every context visited afterwards, so a
// switch always lands on the new context's recorded namespace (D163).
func (contextStateLoader) LoadContextState(name string) tui.ContextState {
	extras, err := loadMenuExtras(name)
	if err != nil {
		slog.Warn("menu config", "context", name, "error", err)
		extras = nil // degrade to the built-in default menu, as launch does.
	}
	state, statePath := loadState(name)
	// Same merge as the launch path (CRD-PIN-01/D193) — a switch must land on the new
	// context's pins, not the departing context's, and the menu extras carried in
	// ContextState are what the post-switch menu rebuild folds in (D163).
	st := tui.ContextState{
		MenuExtras: mergeMenuExtras(extras, state.PinnedResources),
		Namespace:  state.LastNamespace,
	}
	if statePath != "" {
		// Guarded so the interface field stays a true nil when the state path is
		// unresolvable — a typed nil pointer in it would read as "persistence wired"
		// and panic on the first write.
		st.Persister = &statePersister{path: statePath, state: state}
	}
	return st
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

// mergeMenuExtras folds the context's pinned kinds (State.PinnedResources, D193) in
// behind its authored menu entries (menus/<context>.yaml, D83) — the single list the
// shell hands to menu.AddExtras. Authored entries come first and win: where both name
// the same GVR, the hand-written entry keeps its section/title, because that one was
// deliberately written while the pin was recorded automatically. The dedupe is
// deliberately redundant with AddExtras's own GVR dedupe — it is what makes the
// precedence between the two files explicit and testable here, rather than an
// accident of which list happens to be scanned first.
//
// It never returns the caller's backing array: appending pins to the authored slice
// in place would let a later append mutate what the menu file loaded.
func mergeMenuExtras(authored, pinned []config.MenuResource) []config.MenuResource {
	if len(pinned) == 0 {
		return authored
	}
	out := make([]config.MenuResource, 0, len(authored)+len(pinned))
	out = append(out, authored...)
	for _, p := range pinned {
		dup := false
		for _, a := range authored {
			if a.Group == p.Group && a.Version == p.Version && a.Resource == p.Resource {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
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

// resolveTheme turns the config's `theme:` name into the palette the shell renders
// through (M4-12a). An empty name — the zero value, and what an absent key decodes
// to — is the built-in default and never an error, so a config with no theme key is
// silent. An unknown name returns the default *and* an error naming the built-ins:
// the caller keeps launching on the default and reports it once (principle 3), since
// styles.ByName deliberately refuses to guess which theme a typo meant (D169 pt 2).
func resolveTheme(name string) (styles.Theme, error) {
	if strings.TrimSpace(name) == "" {
		return styles.DefaultTheme(), nil
	}
	t, ok := styles.ByName(name)
	if !ok {
		return styles.DefaultTheme(), fmt.Errorf("unknown theme %q, using %q (available: %s)",
			name, styles.DefaultTheme().Name, strings.Join(styles.ThemeNames(), ", "))
	}
	return t, nil
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

// configThemePersister is the launcher's theme-persistence seam (tui.ThemePersister):
// it records a theme picked in the UI as the `theme:` field of the user's config, so
// the next launch resolves it through resolveTheme (M4-12b-2). It is bound to the
// resolved config path at construction, so the tui package stays storage-agnostic —
// the same shape statePersister has for the namespace.
type configThemePersister struct {
	path string
}

// PersistTheme rewrites the config with theme: <name>, keeping every other setting.
//
// It **re-reads the file** rather than holding the config loaded at startup, for two
// reasons: SaveFile marshals the whole struct, so anything not in the value written is
// deleted — a stale in-memory copy would silently revert an edit made since launch,
// and a fresh &config.Config{Theme: name} would delete the user's `keys:` section
// outright. A load failure (the file became unparseable while kubecom ran) aborts the
// write instead of overwriting it with defaults: losing a theme choice is recoverable,
// losing a hand-written keymap is not.
//
// What no save can preserve is the file's *comments and formatting* — sigs.k8s.io/yaml
// marshals a struct, not a document — so a hand-edited config comes back canonicalized.
// That is documented in the README beside the picker.
func (p *configThemePersister) PersistTheme(name string) error {
	cfg, err := config.LoadFile(p.path)
	if err != nil {
		return fmt.Errorf("reading config before writing the theme: %w", err)
	}
	cfg.Theme = name
	return cfg.SaveFile(p.path)
}
