package tui

import (
	ui "github.com/gizak/termui/v3"
)

type Screen struct {
	*ui.Grid
	menu           *MenuList
	rightPaneStack []Pane
	focusStack     []Pane
	focus          Pane
}

func NewScreen() *Screen {
	s := &Screen{
		Grid: ui.NewGrid(),
	}
	return s
}

func (s *Screen) Draw(buf *ui.Buffer) {
	s.setGrid()
	s.Grid.Draw(buf)
}

func (s *Screen) Init() {
	termWidth, termHeight := ui.TerminalDimensions()
	s.SetRect(0, 0, termWidth, termHeight)
}

func (s *Screen) SetMenu(menu *MenuList) {
	s.menu = menu
}

func (s *Screen) setGrid() {
	s.Items = []*ui.GridItem{}
	var right interface{}
	if len(s.rightPaneStack) > 0 {
		right = s.rightPaneStack[0]
	}
	s.Set(
		ui.NewRow(1.0,
			ui.NewCol(0.1, s.menu),
			ui.NewCol(0.9, right),
		),
	)
}

func (s *Screen) AddRightPane(pane Pane) {
	s.rightPaneStack = append([]Pane{pane}, s.rightPaneStack...)
}

func (s *Screen) SetRightPane(pane Pane) {
	s.rightPaneStack = []Pane{pane}
}

func (s *Screen) Focus(focusable Pane) {
	if s.focus != nil {
		s.focus.OnFocusOut()
		s.focusStack = append([]Pane{s.focus}, s.focusStack...)
	}
	s.focus = focusable
	s.focus.OnFocusIn()
}

func (s *Screen) PopFocus() bool {
	if len(s.focusStack) == 0 {
		return false
	}
	if s.focus != nil {
		s.focus.OnFocusOut()
	}
	var next Pane
	if len(s.rightPaneStack) > 1 {
		next = s.rightPaneStack[1]
		s.rightPaneStack = s.rightPaneStack[1:]
	} else {
		next = s.focusStack[0]
	}
	s.focus = next
	s.focus.OnFocusIn()
	s.focusStack = s.focusStack[1:]
	return true
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
	case "<Escape>":
		return s.PopFocus(), false
	default:
		if s.focus != nil {
			return s.focus.OnEvent(event), false
		}
		return false, false
	}
}
