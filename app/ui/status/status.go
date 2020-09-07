package status

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
	"time"
)

type Status struct {
	*focus.Focusable
	*views.Text
	screen  commander.ScreenHandler
	events  chan *tcell.EventKey
	clearIn time.Time
	mu      sync.Mutex
}

func (s *Status) watch() {
	ticker := time.NewTicker(time.Millisecond * 100)
	for {
		t := <-ticker.C
		s.mu.Lock()
		if !s.clearIn.IsZero() && s.clearIn.Before(t) {
			s.Clear()
		}
		s.mu.Unlock()
	}
}

func (s *Status) Clear() {
	s.clearIn = time.Time{}
	s.SetText("")
	s.SetStyle(s.screen.Theme().GetStyle("status-bar"))
	s.screen.UpdateScreen()
}

func (s *Status) ClearIn(duration time.Duration) {
	s.mu.Lock()
	s.clearIn = time.Now().Add(duration)
	s.mu.Unlock()
}

func (s *Status) Warning(msg string) {
	s.Clear()
	s.SetText(msg)
	s.SetStyle(s.screen.Theme().GetStyle("status-warning"))
	s.screen.UpdateScreen()
	s.ClearIn(time.Second * 5)
}

func (s *Status) Info(msg string) {
	s.Clear()
	s.SetText(msg)
	s.SetStyle(s.screen.Theme().GetStyle("status-info"))
	s.screen.UpdateScreen()
	s.ClearIn(time.Second * 2)
}

func (s *Status) Error(err error) {
	s.Clear()
	s.SetText(err.Error())
	s.SetStyle(s.screen.Theme().GetStyle("status-error"))
	s.screen.UpdateScreen()
	s.ClearIn(time.Second * 10)
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
	s.Clear()
	s.SetText(msg)
	s.SetStyle(s.screen.Theme().GetStyle("status-confirm"))
	s.screen.UpdateScreen()
	ev := <-s.events
	switch ev.Rune() {
	case 'y', 'Y':
		return true
	}
	return false
}

func (s *Status) Draw() {
	if s.Style() == tcell.StyleDefault {
		s.SetStyle(s.screen.Theme().GetStyle("status-bar"))
	}
	s.Text.Draw()
}

func (s *Status) Size() (int, int) {
	return 1, 1
}

func NewStatus(screen commander.ScreenHandler) *Status {
	s := &Status{
		Focusable: focus.NewFocusable(),
		Text:      views.NewText(),
		screen:    screen,
		events:    make(chan *tcell.EventKey),
	}
	go s.watch()
	return s
}
