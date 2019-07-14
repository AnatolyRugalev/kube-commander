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
	dur := time.Since(startTime).Round(time.Second)
	if dur > time.Hour*24 {
		days := dur.Nanoseconds() / (time.Hour * 24).Nanoseconds()
		return fmt.Sprintf("%dd", days)
	}
	if dur > time.Hour {
		hours := dur.Nanoseconds() / time.Hour.Nanoseconds()
		return fmt.Sprintf("%dh", hours)
	}
	if dur > time.Minute {
		minutes := dur.Nanoseconds() / time.Minute.Nanoseconds()
		return fmt.Sprintf("%dm", minutes)
	}
	return dur.String()
}

func maxLineWidth(arr []string) int {
	var maxValue int
	for i := 0; i < len(arr); i++ {
		if len(arr[i]) > maxValue {
			maxValue = len(arr[i])
		}
	}
	return maxValue
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
