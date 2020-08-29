package status

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Status struct {
	*focus.Focusable
	*views.Text
	updater commander.ScreenUpdater
	events  chan *tcell.EventKey
}

func (s Status) Error(err error) {
	s.SetText(err.Error())
	s.SetStyle(theme.Default.Foreground(tcell.ColorDarkRed))
	s.updater.UpdateScreen()
}

func (s Status) Warning(msg string) {
	s.SetText(msg)
	s.SetStyle(theme.Default.Foreground(tcell.ColorOrange))
	s.updater.UpdateScreen()
}

func (s Status) Info(msg string) {
	s.SetText(msg)
	s.SetStyle(theme.Default.Foreground(tcell.ColorYellow))
	s.updater.UpdateScreen()
}

func (s Status) HandleEvent(ev tcell.Event) bool {
	if e, ok := ev.(*tcell.EventKey); ok {
		select {
		case s.events <- e:
			return true
		default:
		}
	}
	return false
}

func (s Status) Confirm(msg string) bool {
	s.SetText(msg)
	s.SetStyle(theme.Default.Foreground(tcell.ColorYellow))
	s.updater.UpdateScreen()
	ev := <-s.events
	switch ev.Rune() {
	case 'y', 'Y':
		return true
	}
	return false
}

func (s Status) Size() (int, int) {
	return 1, 1
}

func NewStatus(updater commander.ScreenUpdater) *Status {
	s := &Status{
		Focusable: focus.NewFocusable(),
		Text:      views.NewText(),
		updater:   updater,
		events:    make(chan *tcell.EventKey),
	}
	s.SetStyle(theme.Default.Background(theme.ColorDisabledForeground))
	return s
}
