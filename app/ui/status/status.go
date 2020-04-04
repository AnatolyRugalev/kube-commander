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
}

func (s Status) Error(err error) {
	s.SetText(err.Error())
	s.SetStyle(theme.Default.Foreground(tcell.ColorDarkRed))
	s.updater.UpdateScreen()
}

func (s Status) Info(msg string) {
	s.SetText(msg)
	s.SetStyle(theme.Default.Foreground(tcell.ColorYellow))
	s.updater.UpdateScreen()
}

func (s Status) Size() (int, int) {
	return 1, 1
}

func NewStatus(updater commander.ScreenUpdater) *Status {
	s := &Status{
		Focusable: focus.NewFocusable(),
		Text:      views.NewText(),
		updater:   updater,
	}
	s.SetStyle(theme.Default)
	return s
}
