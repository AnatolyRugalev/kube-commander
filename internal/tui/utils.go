package tui

import "time"

func Age(startTime time.Time) string {
	// TODO: humanize
	return time.Since(startTime).Round(time.Second).String()
}
