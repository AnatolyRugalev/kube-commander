package main

import (
	"fmt"
	"log/slog"
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
	kubeconfig string // --kubeconfig: explicit kubeconfig path ("" = standard rules)
	context    string // --context: kubeconfig context name ("" = current-context)
	namespace  string // -n/--namespace: initial watch scope ("" = all namespaces)
	configPath string // --config: kubecom config file ("" = user config dir)
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
	var startupErr *tui.ErrorMsg
	if menuErr != nil {
		slog.Warn("menu config", "context", ctxName, "error", menuErr)
		e := tui.NewErrorMsg("menu config", menuErr)
		startupErr = &e
	}

	// Construct the shell over the resolved keymap with the live client wired in for
	// watches and discovery, scoped to the requested namespace. The model requests
	// the alternate screen itself (via View.AltScreen — D70), so no program option
	// is needed here.
	model := tui.NewWithKeymap(km,
		tui.WithWatcher(clients),
		tui.WithDiscoverer(clients),
		tui.WithNamespaceLister(clients),
		tui.WithNamespace(opts.namespace),
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
