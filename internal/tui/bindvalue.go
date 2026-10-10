package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/neuroplastio/kubecom/internal/kube"
	"github.com/neuroplastio/kubecom/internal/tui/keymap"
	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// handleBound runs what a key resolved to: its action, or — when the binding gives a
// value (`bind "g p" "resources.switch" "pods"`) — the argument verb run with that
// value, with no palette (D303 pt 2).
//
// The value runs exactly where the key without one would open the verb's palette
// stage, so every surface keeps its say: a view that swallows `R` swallows `g p`
// too, and the logs view that lets `N` through lets a namespace key through. That
// is decided by the same routing (handleAction → openPaletteArg), which finds the
// value on the model for the length of this one call and runs it instead of opening
// the stage.
func (m Model) handleBound(a keymap.Action, args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m.handleAction(a)
	}
	v := args[0]
	m.bindValue = &v
	next, cmd := m.handleAction(a)
	if nm, ok := next.(Model); ok {
		nm.bindValue = nil
		next = nm
	}
	return next, cmd
}

// takeBindValue reports the value of the binding being handled, if any, and clears
// it, so it runs once.
func (m *Model) takeBindValue() (string, bool) {
	if m.bindValue == nil {
		return "", false
	}
	v := *m.bindValue
	m.bindValue = nil
	return v, true
}

// runArgument runs an argument verb with a binding's value: the palette line
// `:<verb> <value>` committed without the palette. Each arm ends where the palette's
// commit does (applyPaletteArg) and is inert where the verb's stage would be
// (enterPaletteArg). A value that names nothing here says so on the status line
// rather than doing nothing silently.
func (m Model) runArgument(a keymap.Action, value string) (tea.Model, tea.Cmd) {
	switch a {
	case keymap.ActionResources:
		if m.watcher == nil {
			return m, nil
		}
		r, ok := m.resourceNamed(value)
		if !ok {
			return m, m.surfaceNotice(noSuchValue(a, value))
		}
		return m.selectResource(r)
	case keymap.ActionPin:
		if m.pinner == nil {
			return m, nil
		}
		r, ok := m.resourceNamed(value)
		if !ok {
			return m, m.surfaceNotice(noSuchValue(a, value))
		}
		return m.togglePin(r)
	case keymap.ActionTheme:
		t, ok := styles.ByName(value)
		if !ok {
			return m, m.surfaceNotice(noSuchValue(a, value))
		}
		return m.applyThemeNamed(t.Name)
	case keymap.ActionNamespace:
		if m.nsLister == nil {
			return m, nil
		}
		// "" is all namespaces, as it is for -n.
		return m.applyNamespaceValue(value)
	case keymap.ActionContext:
		if m.ctxLister == nil {
			return m, nil
		}
		// A context the kubeconfig does not have fails the connect, which says so.
		return m.switchContext(value)
	case keymap.ActionActions:
		if m.logsView.Active() || !m.hasCurrent {
			return m, nil
		}
		titles, byTitle := rowActionTitles(m.current)
		for _, t := range titles {
			if act := byTitle[t]; strings.EqualFold(t, value) || strings.EqualFold(string(act), value) {
				return m.dispatchRowAction(act)
			}
		}
		return m, m.surfaceNotice(noSuchValue(a, value))
	}
	return m, nil
}

// noSuchValue is the notice for a binding's value that names nothing here.
func noSuchValue(a keymap.Action, value string) string {
	return fmt.Sprintf("%s: nothing here is called %q", a, value)
}

// resourceNamed is the kind a binding's value names, as `:resource ` would find it:
// its Kind as listed (`Pod`, or `Cluster (postgresql.cnpg.io)` where two groups
// share one), its plural, a short name, or kubectl's `<plural>.<group>`. A name
// more than one kind answers to — an API group — names none.
func (m Model) resourceNamed(value string) (kube.Resource, bool) {
	items, byLabel := m.resourcePickerItems()
	var found []kube.Resource
	for _, it := range items {
		r := byLabel[it.Label]
		if strings.EqualFold(it.Label, value) || strings.EqualFold(r.GVR.Resource+"."+r.GVR.Group, value) {
			return r, true
		}
		for _, alias := range it.Aliases {
			if strings.EqualFold(alias, value) {
				found = append(found, r)
				break
			}
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return kube.Resource{}, false
}
