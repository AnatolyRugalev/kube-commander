// Package filter is the browse table's live filter field (M2-09b): the `/`-prompted
// text input the root shell opens over the current resource table, where typing
// narrows the displayed rows live. It is the M2-09b half of the root shell moved
// into a self-contained components/* sub-model (D264/MONO-01), following the seam
// D265 settled for a shell-owned surface: the shell keeps the authoritative rows
// (the table and its full set) and performs the narrowing (`table.SetFilter`), while
// this model owns only its own state — whether the field is open and the query text
// (the textinput's value, cursor and focus) — plus the live prompt render the status
// bar shows while editing.
//
// No view matches a raw key (D11): the control/text split stays in the shell's
// `routeFilterKey`, which resolves a mapped no-text key as a control action and feeds
// every remaining text-carrying key to this model's Update. The shell is also where
// the field's status-bar indicator and hint context come from; here the model is pure
// state and rendering.
package filter

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Model is the browse filter field. Every field is owned by the embedding root
// model; nothing here is shared across goroutines (principle 1).
type Model struct {
	input textinput.Model
	open  bool // whether the field is open and capturing text — "" View when false
}

// New builds a closed filter field with the `/` prompt every `/` surface uses.
func New() Model {
	fi := textinput.New()
	fi.Prompt = "/"
	return Model{input: fi}
}

// Active reports whether the field is open and capturing input.
func (m Model) Active() bool { return m.open }

// Value is the current query text ("" when the field is closed and reset).
func (m Model) Value() string { return m.input.Value() }

// View renders the live prompt the status bar shows while editing ("/query" plus
// the cursor). Meaningful only while the field is open.
func (m Model) View() string { return m.input.View() }

// Open shows the field over the initial query with the cursor at the end. The
// shell seeds it with any already-active filter, so reopening `/` edits the current
// query. Returns the Focus command the field needs to capture the keyboard.
func (m Model) Open(initial string) (Model, tea.Cmd) {
	m.open = true
	m.input.SetValue(initial)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// Commit closes the field while keeping the query text: the narrowed view stays
// applied, the value is what a later open seeds itself with, and the status bar
// switches from the live prompt to the committed "/query" indicator.
func (m Model) Commit() Model {
	m.open = false
	m.input.Blur()
	return m
}

// Reset closes the field and clears the query — the teardown a fresh resource, a
// context switch, or a cancel (esc) uses, returning the table to its full row set.
func (m Model) Reset() Model {
	m.open = false
	m.input.Blur()
	m.input.Reset()
	return m
}

// Update feeds one keypress to the text field and returns the command it issues.
// Only text-carrying keys reach here: the shell's routeFilterKey has already
// resolved every mapped no-text key as a control action (D73). A no-op — the
// input's own Update — against the field's current state.
func (m Model) Update(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
