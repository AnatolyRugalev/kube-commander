package tui

import (
	ui "github.com/gizak/termui/v3"
)

type Screen struct {
	*ui.Grid
	focus Focusable
}

func NewScreen() *Screen {
	s := &Screen{
		Grid: ui.NewGrid(),
	}
	return s
}

func (s *Screen) Init() {
	termWidth, termHeight := ui.TerminalDimensions()
	s.SetRect(0, 0, termWidth, termHeight)
}

func (s *Screen) SetPanes(left *MenuList, right interface{}) {
	s.Items = []*ui.GridItem{}
	s.Set(
		ui.NewRow(1.0,
			ui.NewCol(0.1, left),
			ui.NewCol(0.9, right),
		),
	)
}

func (s *Screen) Focus(focusable Focusable) {
	if s.focus != nil {
		s.focus.OnFocusOut()
	}
	s.focus = focusable
	s.focus.OnFocusIn()
}

func (s *Screen) OnEvent(event *ui.Event) (bool, bool) {
	switch event.ID {
	case "q", "<C-c>":
		return false, true
	case "<Resize>":
		payload := event.Payload.(ui.Resize)
		s.SetRect(0, 0, payload.Width, payload.Height)
		ui.Clear()
		return true, false
	default:
		if s.focus != nil {
			return s.focus.OnEvent(event), false
		} else {
			return false, false
		}
	}
}
