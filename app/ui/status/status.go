package status

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

type Status struct {
	*focus.Focusable
	*views.Text
	updater commander.ScreenUpdater
	events  chan *tcell.EventKey
	style   tcell.Style
	clearIn time.Time
	mu      sync.Mutex
}

func (s *Status) Error(err error) {
	s.SetText(err.Error())
	s.SetStyle(s.style.Foreground(tcell.ColorDarkRed))
	s.updater.UpdateScreen()
	s.ClearIn(time.Second * 10)
}

func (s *Status) watch() {
	ticker := time.NewTicker(time.Millisecond * 100)
	for {
		t := <-ticker.C
		s.mu.Lock()
		if !s.clearIn.IsZero() && s.clearIn.Before(t) {
			s.Clear()
			s.clearIn = time.Time{}
		}
		s.mu.Unlock()
	}
}

func (s *Status) Clear() {
	s.SetText("")
	s.SetStyle(s.style)
	s.updater.UpdateScreen()
}

func (s *Status) ClearIn(duration time.Duration) {
	s.mu.Lock()
	s.clearIn = time.Now().Add(duration)
	s.mu.Unlock()
}

func (s *Status) Warning(msg string) {
	s.SetText(msg)
	s.SetStyle(s.style.Foreground(tcell.ColorOrange))
	s.updater.UpdateScreen()
	s.ClearIn(time.Second * 5)
}

func (s *Status) Info(msg string) {
	s.SetText(msg)
	s.SetStyle(s.style.Foreground(tcell.ColorYellow))
	s.updater.UpdateScreen()
	s.ClearIn(time.Second * 2)
}

func (s *Status) HandleEvent(ev tcell.Event) bool {
	if e, ok := ev.(*tcell.EventKey); ok {
		select {
		case s.events <- e:
			return true
		default:
		}
	}
	return false
}

func (s *Status) Confirm(msg string) bool {
	s.SetText(msg)
	s.SetStyle(s.style.Foreground(tcell.ColorYellow))
	s.updater.UpdateScreen()
	ev := <-s.events
	switch ev.Rune() {
	case 'y', 'Y':
		return true
	}
	return false
}

func (s *Status) Size() (int, int) {
	return 1, 1
}

func NewStatus(updater commander.ScreenUpdater) *Status {
	s := &Status{
		Focusable: focus.NewFocusable(),
		Text:      views.NewText(),
		updater:   updater,
		events:    make(chan *tcell.EventKey),
		style:     theme.Default.Background(theme.ColorDisabledForeground),
	}
	s.SetStyle(s.style)
	go s.watch()
	return s
}
