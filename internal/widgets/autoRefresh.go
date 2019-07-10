package widgets

import (
	"github.com/gizak/termui/v3"
	"time"
)

type AutoRefresh struct {
	*termui.Block
	currentInterval int

	timer *time.Timer
}

type Refreshable interface {
	Refresh()
}

var refreshIntervals = []time.Duration{
	0,
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

func NewAutoRefresh(refreshable Refreshable) *AutoRefresh {
	block := termui.NewBlock()
	block.Border = false
	ar := &AutoRefresh{
		Block: block,
		timer: time.NewTimer(refreshIntervals[0]),
	}
	ar.resetTimer()
	go func() {
		for {
			<-ar.timer.C
			refreshable.Refresh()
			ar.resetTimer()
		}
	}()
	return ar
}

func (ar *AutoRefresh) Toggle() {
	nextInterval := ar.currentInterval + 1
	if nextInterval > len(refreshIntervals)-1 {
		nextInterval = 0
	}
	ar.currentInterval = nextInterval
	ar.resetTimer()
}

func (ar *AutoRefresh) Interval() time.Duration {
	return refreshIntervals[ar.currentInterval]
}

func (ar *AutoRefresh) resetTimer() {
	duration := refreshIntervals[ar.currentInterval]
	if duration == 0 {
		ar.timer.Stop()
	} else {
		ar.timer.Reset(duration)
	}
}
