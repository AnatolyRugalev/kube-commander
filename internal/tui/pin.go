package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/AnatolyRugalev/kube-commander/internal/config"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/components/menu"
)

// The pin gesture (CRD-PIN-02, the write half of the CRD-PIN line): `*` on a kind
// records it in the current context's state file, and it joins that context's menu
// from then on — the answer to the feedback's "a kind you reach for once should be
// in your menu from then on" (D193).
//
// The shape follows the seams already here: the tui package writes no files, so the
// pin goes out through a persister the launcher binds to one context's state path
// (PinPersister — the statePersister/configThemePersister shape), and the row lands
// in the live menu through the same menu.AddExtras the launch path folds pins in
// with, so a pinned row and a hand-written menus/<context>.yaml row are one code
// path (D193 pt 2) and dedupe by GVR against seed, extras and discovery alike.
//
// CRD-PIN-03 adds the unpin half to this same action and key; the action is named
// `menu.pin` from the start — a toggle by intent — so that slice does not have to
// rename a user-bindable id out from under anyone's keymap.

// PinPersister records a kind as pinned for the active kubeconfig context so the
// next launch — and every context switch back — shows it in the menu (CRD-PIN-02).
// It is the write side of State.PinnedResources (config/state.go, D193): the
// launcher, which alone knows the resolved context and its state-file path, wires a
// persister already bound to that context, so the tui package stays storage- and
// context-agnostic, exactly as NamespacePersister does for the namespace.
//
// A model built without one (the default, or an unresolved context) is pin-inert:
// the action is a no-op rather than a pin that silently fails to survive the session.
type PinPersister interface {
	PersistPin(r config.MenuResource) error
}

// WithPinPersister wires the per-context state writer the shell calls when the user
// pins a kind (CRD-PIN-02). Without it (or with a nil persister) the pin action is
// inert — nothing is written and no row is added, so the menu never claims a
// persistence the shell cannot deliver.
func WithPinPersister(p PinPersister) Option {
	return func(m *Model) { m.pinner = p }
}

// pinTarget resolves the kind the pin gesture acts on from whichever browse surface
// can name one, and reports whether there is one at all:
//
//   - the table, when it holds focus and a kind is open — the kind being browsed,
//     which is where a reader who arrived from a search hit or the resource picker
//     ends up ("I want to keep *this*");
//   - otherwise the highlighted menu row, when it is a resource row (the namespace
//     seam names no kind).
//
// Focus is the whole rule, deliberately: it is what the reader is pointing at, and
// it is the same rule nav.up/nav.down already follow, so the gesture needs no
// explanation beyond "pins what you are on". An unavailable menu row (a group that
// failed discovery, D57) is still pinnable — a kind whose group is down right now is
// precisely one you may want kept in the menu for when it comes back.
func (m Model) pinTarget() (kube.Resource, bool) {
	if m.table.Focused() && m.hasCurrent {
		return m.current, true
	}
	it, ok := m.menu.Selected()
	if !ok || it.Kind != menu.ItemResource {
		return kube.Resource{}, false
	}
	return it.Resource, true
}

// pinResource is the menu.pin handler: it resolves the kind under the cursor,
// records it through the persister, and folds it into the live menu so the row
// appears without a relaunch.
//
// The "already there" test is against m.menuExtras — the merged authored + pinned
// list this context launched (or switched) with — and *not* against the menu's
// items, which is the tempting and wrong source. Discovery appends every kind it
// finds to the menu, so testing the menu would make pinning a discovered CRD a
// no-op, which is exactly the case the whole line exists for: a pin is what keeps
// the kind in the menu when discovery does not list it (a narrowed menu, a failed
// API group, a launch before the pass returns). Testing the extras list instead
// makes the no-op mean what the board asked for — the kind is already an entry, so
// pinning it again would be a duplicate row — and it stays correct across a context
// switch, since m.menuExtras is rebound to the new context's list (D163/D193 pt 4).
//
// The write runs off the update loop (a tea.Cmd) like every other persistence here;
// a failure surfaces as a transient toast and the row stays for the session
// (principle 3 — the choice applies, it just may not survive the process).
func (m Model) pinResource() (tea.Model, tea.Cmd) {
	if m.pinner == nil {
		return m, nil // pin-inert: no persister, no gesture.
	}
	r, ok := m.pinTarget()
	if !ok {
		return m, nil
	}
	entry := pinEntry(r)
	label := pinLabel(r)
	for _, e := range m.menuExtras {
		if e.Group == entry.Group && e.Version == entry.Version && e.Resource == entry.Resource {
			notice := m.surfaceNotice(label + " is already in the menu")
			return m, notice
		}
	}

	// Menu first, then the write: the row is what the reader asked for, and it must
	// not wait on a file. AddExtras dedupes by GVR, so a kind discovery already lists
	// keeps its single row and simply gains a pin behind it.
	m.menuExtras = append(append([]config.MenuResource(nil), m.menuExtras...), entry)
	m.menu.AddExtras([]config.MenuResource{entry})
	notice := m.surfaceNotice("pinned " + label)
	return m, tea.Batch(notice, m.persistPin(entry))
}

// persistPin writes one pinned kind to the context's state file off the update loop,
// so persistence never blocks input (the persistNamespace shape). A write failure is
// a transient toast, not a fatal: the row is already in the menu for this session.
func (m Model) persistPin(entry config.MenuResource) tea.Cmd {
	p := m.pinner
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		if err := p.PersistPin(entry); err != nil {
			return NewErrorMsg("persist pin", err)
		}
		return nil
	}
}

// pinEntry maps a discovered kube.Resource to the config.MenuResource the state file
// stores and the menu folds in. Group/Version/Resource are the pin's identity (D193
// pt 5); Kind rides along so the row has a legible title without discovery having run
// (menu.extraItem falls back Title → Kind → Resource), and Namespaced comes from the
// resource discovery reported rather than a guess — a namespaced CRD pinned as
// cluster-scoped lists nothing. Section and Title are left empty on purpose: an
// automatic pin lands in the same "Custom Resources" bucket discovered CRDs do, and
// naming a section is the hand-authored file's business (D193 pt 3).
func pinEntry(r kube.Resource) config.MenuResource {
	return config.MenuResource{
		Group:      r.GVR.Group,
		Version:    r.GVR.Version,
		Resource:   r.GVR.Resource,
		Kind:       r.GVK.Kind,
		Namespaced: r.Namespaced,
	}
}

// pinLabel is what the status-bar notice calls the kind: its Kind ("ExternalSecret"),
// falling back to the plural resource name for a row that has no Kind — a
// hand-written menu entry may omit it, and a notice naming nothing is worse than one
// naming the resource.
func pinLabel(r kube.Resource) string {
	if r.GVK.Kind != "" {
		return r.GVK.Kind
	}
	return r.GVR.Resource
}
