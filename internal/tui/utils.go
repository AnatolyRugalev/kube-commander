package tui

import (
	"time"

	ui "github.com/gizak/termui/v3"
)

const (
	eventMouseLeftDouble = "<MouseLeftDouble>"
	doubleClickSensitive = 200
)

func Age(startTime time.Time) string {
	// TODO: humanize
	return time.Since(startTime).Round(time.Second).String()
}

func cloneEvent(e *ui.Event, newID string) *ui.Event {
	newEvent := &ui.Event{Type: e.Type, ID: newID, Payload: e.Payload}
	return newEvent
}
