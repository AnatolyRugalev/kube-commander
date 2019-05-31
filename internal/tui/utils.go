package tui

import "time"

const (
	eventMouseLeftDouble = "<MouseLeftDouble>"
	doubleClickSensitive = 300
)

func Age(startTime time.Time) string {
	// TODO: humanize
	return time.Since(startTime).Round(time.Second).String()
}
