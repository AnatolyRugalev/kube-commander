package tui

import (
	"github.com/neuroplastio/kubecom/internal/keybind"
	"github.com/neuroplastio/kubecom/internal/tui/components/searchview"
)

// pickerIDs names each picker as a context reads it (`picker.id == 'ports'`).
var pickerIDs = map[string]string{
	commandPickerKind:   "palette",
	containerPickerKind: "containers",
	portPickerKind:      "ports",
	relationPickerKind:  "relations",
}

// keyPath is what holds the keyboard, as the keymap's contexts read it (D303 pt 3):
// the cluster, the pane with focus, its sort mode, the view opened over it, the
// overlay over that, and a text field. Every key resolves in it, and the hint and
// the `?` overlay read their keys from it, so a key means here what the hint says.
//
// The surfaces are placed in the order the shell routes keys to them. The search
// mini-app, the unhealthy list, the logs view and the viewer each capture every key
// while they are up, and at most one is; a picker, the prompt and the confirm modal
// open over any of them. The port-forward panel and the keybindings overlay capture
// keys only with no view up — a view takes them first (handleAction) — so with a
// view up they are not on the path.
func (m *Model) keyPath() keybind.Path {
	var p keybind.Path
	if m.context != "" {
		p.Cluster = &keybind.Cluster{Context: m.context, Namespace: m.namespace}
	}
	if m.table.Focused() {
		t := &keybind.Table{
			Drilled:       m.hasChildScope,
			Filtered:      m.table.Filter() != "",
			UnhealthyOnly: m.table.UnhealthyOnly(),
		}
		if m.hasCurrent {
			t.Resource, t.Group, t.Kind = m.current.GVR.Resource, m.current.GVR.Group, m.current.GVK.Kind
			t.Namespaced = m.current.Namespaced
		}
		p.Table = t
		p.Sort = m.table.SortMode()
	} else {
		p.Menu = true
	}

	view := true
	switch {
	case m.searchView.Active():
		p.Search = &keybind.Search{AllKinds: m.searchView.AllKinds(), AllNamespaces: m.searchView.AllNamespaces()}
		// The query field is open until the reader moves onto the results (D140).
		p.Field = m.searchView.Focus() != searchview.FocusResults
	case m.unhealthyView.Active():
		p.Unhealthy = true
	case m.logsView.Active():
		p.Logs = &keybind.Logs{
			Follow:     m.logsView.Following(),
			Wrap:       m.logsView.Wrap(),
			Timestamps: m.logsView.Timestamps(),
			Previous:   m.logsView.Previous(),
			Selecting:  m.logsView.Selecting(),
		}
		p.Field = m.logsView.Filtering()
	case m.viewer.Active():
		p.Viewer = &keybind.Viewer{Content: m.viewer.Kind()}
	default:
		view = false
	}

	switch pk := m.activePicker(); {
	case pk != nil:
		p.Picker = &keybind.Picker{ID: pickerIDs[pk.Kind()]}
		if pk.Kind() == commandPickerKind {
			p.Picker.Stage = paletteArgVerbs[m.palArg]
		}
		p.Field = pk.Filtering()
	case m.modal.Prompting():
		p.Prompt, p.Field = true, true
	case m.modal.Active():
		p.Confirm = true
	case view:
	case m.pfPanel.Active():
		p.Forwards = true
	case m.help.Visible():
		p.Help = true
	}
	if m.filter.Active() {
		p.Field = true
	}
	return p
}
