package tui

import (
	"fmt"
	"time"

	ui "github.com/gizak/termui/v3"
)

const (
	eventMouseLeftDouble = "<MouseLeftDouble>"
	doubleClickSensitive = 200
	tiMouseMoveEnable    = "\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h"
	tiMouseMoveDisable   = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"
)

func Age(startTime time.Time) string {
	// TODO: humanize
	return time.Since(startTime).Round(time.Second).String()
}

func mouseMoveEvents(enable bool) {
	if enable {
		fmt.Print(tiMouseMoveEnable)
		return
	}
	fmt.Print(tiMouseMoveDisable)
}

func cloneEvent(e *ui.Event, newID string) *ui.Event {
	newEvent := &ui.Event{Type: e.Type, ID: newID, Payload: e.Payload}
	return newEvent
}
