package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// This file is AGE-01: the clock that keeps the browse table's AGE column moving
// while the reader sits still.
//
// The column's staleness is not a refresh bug — the watch is working, and the
// cell is exactly what the server printed. It is that the server printed it
// *once*: a row is only re-rendered when a delta arrives for it, and an object
// that nothing modifies produces no deltas, so its age is pinned to whenever it
// was last listed. table.RefreshAges re-derives the cell locally; this is the
// heartbeat that calls it.
//
// The tick is unconditional and self-perpetuating — armed by Init, re-armed by
// its own handler, never gated on there being a table, an age column or a
// cluster. That is deliberate. A gated tick needs a generation tag and a restart
// on every kind change, namespace re-scope, drill-down, reconnect and context
// switch (metrics.go carries exactly that machinery, and it needs it because each
// tick issues a *request*). This one touches nothing outside the model: on a tick
// where no age string changed, RefreshAges compares n timestamps and returns.
// Paying that once a second is cheaper than a state machine that can get stuck
// off — the failure mode this feedback reported in the first place.

// ageInterval is how often the age column is re-derived. One second is the
// finest granularity the format has — `duration.HumanDuration` counts seconds
// below two minutes — so a slower tick would visibly skip numbers on a
// just-created object, which is the moment a reader is most likely watching the
// column.
const ageInterval = time.Second

// ageTickMsg is the heartbeat. It carries nothing: unlike the metrics poll there
// is no in-flight work to invalidate, so there is no generation to check and a
// tick is never stale.
type ageTickMsg struct{}

// scheduleAgeTick arms the next beat.
func scheduleAgeTick() tea.Cmd {
	return tea.Tick(ageInterval, func(time.Time) tea.Msg { return ageTickMsg{} })
}

// handleAgeTick re-derives the ages and re-arms. It reads the wall clock here,
// at the one seam that has to, so everything below it stays a function of a
// passed-in time and is testable without sleeping.
func (m Model) handleAgeTick() (tea.Model, tea.Cmd) {
	m.table.RefreshAges(time.Now())
	return m, scheduleAgeTick()
}
