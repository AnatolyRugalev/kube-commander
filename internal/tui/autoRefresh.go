package tui

import (
	"time"
)

var refreshIntervals = []time.Duration{
	0,
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

func (s *Screen) resetRefreshTimer() {
	duration := refreshIntervals[s.refreshInterval]
	if duration == 0 {
		s.refreshTimer.Stop()
	} else {
		s.refreshTimer.Reset(duration)
	}
}

func (s *Screen) startAutoRefresh() {
	duration := refreshIntervals[s.refreshInterval]
	if duration == 0 {
		s.refreshTimer = time.NewTimer(time.Hour)
		s.refreshTimer.Stop()
	} else {
		s.refreshTimer = time.NewTimer(duration)
	}
	go func() {
		for {
			s.resetRefreshTimer()
			<-s.refreshTimer.C
			s.reloadCurrentRightPane()
		}
	}()
}

func (s *Screen) toggleAutoRefresh() {
	nextInterval := s.refreshInterval + 1
	if nextInterval > len(refreshIntervals)-1 {
		nextInterval = 0
	}
	s.refreshInterval = nextInterval
	s.resetRefreshTimer()
}
