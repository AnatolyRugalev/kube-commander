package tui

import (
	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

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
// in the live menu through the same insertion the launch path folds pins in with
// (menu.AddPinned, AddExtras's twin), so a pinned row and a hand-written
// menus/<context>.yaml row render through one code path and dedupe by GVR against
// seed, extras and discovery alike.
//
// CRD-PIN-03 made the same action and key the *unpin* too — `menu.pin` was named as
// a toggle by intent, so nothing user-bindable had to be renamed. What the toggle
// needed was provenance: a row the shell may remove is one a pin put there, so the
// pinned kinds arrive as their own list (WithPinnedResources) rather than pre-merged
// into the authored ones, and the menu marks the rows it inserts for them (D202).

// PinPersister records — and, since CRD-PIN-03, removes — a kind pinned for the
// active kubeconfig context so the next launch, and every context switch back, shows
// it in the menu (or stops showing it). It is the write side of State.PinnedResources
// (config/state.go, D193): the launcher, which alone knows the resolved context and
// its state-file path, wires a persister already bound to that context, so the tui
// package stays storage- and context-agnostic, exactly as NamespacePersister does for
// the namespace.
//
// The two halves are one interface on purpose: a surface that can pin but not unpin
// is the state CRD-PIN-02 shipped in, and it left the only way back a text editor.
//
// A model built without one (the default, or an unresolved context) is pin-inert:
// the action is a no-op rather than a pin that silently fails to survive the session.
type PinPersister interface {
	PersistPin(r config.MenuResource) error
	// PersistUnpin removes the kind from the context's pinned list. It is given the
	// same MenuResource pinning recorded, and matches it by GVR (the pin's identity,
	// D193 pt 5) — a kind that is not pinned is not an error, it is nothing to do.
	PersistUnpin(r config.MenuResource) error
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

// pinResource is the menu.pin handler, and since CRD-PIN-03 it is a **toggle**: it
// resolves the kind under the cursor and either records it or takes it back out,
// through the persister and in the live menu, so neither direction needs a relaunch
// or a text editor (D202).
//
// Which of the three things happens is decided by *where the kind already is*, in
// this order:
//
//  1. in m.menuExtras — a hand-written entry in `menus/<context>.yaml`. Declined
//     with a notice naming the file: the authored entry wins over a pin (D193 pt 3),
//     so pinning it would record a line that can never render, and unpinning it would
//     have `*` silently disagree with a file the user wrote by hand. Editing that
//     file is the way to remove it, and the notice says so.
//  2. in m.menuPinned — this context's own pins. Unpin: the entry leaves the list,
//     the row leaves the menu unless discovery also lists the kind (menu.Unpin), and
//     the state file loses the line.
//  3. neither — pin it.
//
// Case 3's test is deliberately **not** "is it in the menu", which is the tempting
// and wrong source: discovery appends every kind it finds, so testing the menu would
// make pinning a discovered CRD a no-op — exactly the case the whole line exists for,
// since a pin is what keeps the kind in the menu when discovery does not list it (a
// narrowed menu, a failed API group, a launch before the pass returns). Testing the
// two entry lists instead means what the board asked for — the kind is already an
// entry — and stays correct across a context switch, since both lists are rebound to
// the new context's (D163/D193 pt 4).
//
// The write runs off the update loop (a tea.Cmd) like every other persistence here;
// a failure surfaces as a transient toast and the menu keeps what the reader asked
// for (principle 3 — the choice applies, it just may not survive the process).
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
	if indexOfGVR(m.menuExtras, entry) >= 0 {
		return m, m.surfaceNotice(label + " is in this context's menu file")
	}
	if i := indexOfGVR(m.menuPinned, entry); i >= 0 {
		return m.unpinResource(i, entry, label)
	}

	// Menu first, then the write: the row is what the reader asked for, and it must
	// not wait on a file. AddPinned dedupes by GVR, so a kind discovery already lists
	// keeps its single row and simply gains a pin behind it.
	m.menuPinned = append(append([]config.MenuResource(nil), m.menuPinned...), entry)
	m.menu.AddPinned([]config.MenuResource{entry})
	notice := m.surfaceNotice("pinned " + label)
	return m, tea.Batch(notice, m.persistPin(entry))
}

// unpinResource is the toggle's other half: it drops the pin at index i from the
// live list, asks the menu to take the row with it (menu.Unpin keeps the row when
// discovery lists the kind too), and clears the line from the state file.
//
// The notice is the same either way — "unpinned X" — because that is what the reader
// did; whether the row survives is a fact about the cluster, and it is on screen.
func (m Model) unpinResource(i int, entry config.MenuResource, label string) (tea.Model, tea.Cmd) {
	pins := append([]config.MenuResource(nil), m.menuPinned...)
	m.menuPinned = append(pins[:i], pins[i+1:]...)
	m.menu.Unpin(schema.GroupVersionResource{
		Group: entry.Group, Version: entry.Version, Resource: entry.Resource,
	})
	notice := m.surfaceNotice("unpinned " + label)
	return m, tea.Batch(notice, m.persistUnpin(entry))
}

// indexOfGVR finds entry's group/version/resource in a list of menu entries, or -1.
// GVR is the pin's identity (D193 pt 5) — title, section and the Kind spelling are
// presentation, and two entries differing only in those name the same kind.
func indexOfGVR(list []config.MenuResource, entry config.MenuResource) int {
	for i, e := range list {
		if e.Group == entry.Group && e.Version == entry.Version && e.Resource == entry.Resource {
			return i
		}
	}
	return -1
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

// persistUnpin clears one pinned kind from the context's state file, the mirror of
// persistPin: off the update loop, and a failure is a toast rather than a rollback —
// the row is already gone from the menu for this session, and a rollback would put
// back a row the reader just removed to say the write failed.
func (m Model) persistUnpin(entry config.MenuResource) tea.Cmd {
	p := m.pinner
	if p == nil {
		return nil
	}
	return func() tea.Msg {
		if err := p.PersistUnpin(entry); err != nil {
			return NewErrorMsg("persist unpin", err)
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
