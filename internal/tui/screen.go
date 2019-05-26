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

func (s *Screen) Render() {
	ui.Render(s)
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

	menuRatio := 15.0 / float64(s.Rectangle.Max.X)
	s.Set(
		ui.NewRow(1.0,
			ui.NewCol(menuRatio, s.menu),
			ui.NewCol(1-menuRatio, right),
		),
	)
}

func (s *Screen) Focus(focusable Pane) {
	if s.focus != nil {
		s.focus.OnFocusOut()
		s.focusStack = append([]Pane{s.focus}, s.focusStack...)
	}
	s.focus = focusable
	s.focus.OnFocusIn()
}

func (s *Screen) popFocus() bool {
	if len(s.focusStack) == 0 {
		return false
	}
	if s.focus != nil {
		s.focus.OnFocusOut()
	}
	s.focus = s.focusStack[0]
	s.focus.OnFocusIn()
	s.focusStack = s.focusStack[1:]
	return true
}

func (s *Screen) popRightPane() Pane {
	if len(s.rightPaneStack) == 0 {
		return nil
	}
	if s.rightPaneStack[0] == s.focus {
		s.popFocus()
	}
	var next Pane
	if len(s.rightPaneStack) > 1 {
		next = s.rightPaneStack[1]
		s.rightPaneStack = s.rightPaneStack[1:]
	}
	return next
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
		if s.focus == s.menu {
			return false, false
		}
		if len(s.rightPaneStack) > 1 {
			s.popRightPane()
			return true, false
		} else {
			return s.popFocus(), false
		}
	default:
		if s.focus != nil {
			return s.focus.OnEvent(event), false
		}
		return false, false
	}
}

func (s *Screen) setRightPane(pane Pane) {
	s.rightPaneStack = []Pane{pane}
}

func (s *Screen) appendRightPane(pane Pane) {
	refocus := s.focus == s.rightPaneStack[0]
	s.rightPaneStack = append([]Pane{pane}, s.rightPaneStack...)
	if refocus {
		s.Focus(s.rightPaneStack[0])
	}
}

func (s *Screen) LoadRightPane(pane Pane) {
	s.appendRightPane(pane)
	if _, ok := pane.(Loadable); ok {
		s.reloadCurrentRightPane()
	}
}

func (s *Screen) ReplaceRightPane(pane Pane) {
	s.setRightPane(pane)
	if _, ok := pane.(Loadable); ok {
		s.reloadCurrentRightPane()
	}
}

func (s *Screen) reloadCurrentRightPane() {
	pane := s.rightPaneStack[0].(Loadable)
	preloader := NewPreloader()
	// Add preloader overlay
	s.appendRightPane(preloader)
	s.Render()
	go func() {
		err := pane.Reload()
		if err != nil {
			ShowDialog("Error", err.Error(), ButtonOk, ButtonCancel)
		} else {
			// Remove preloader overlay
			s.popRightPane()
		}
		s.Render()
	}()
}
