package tui

import (
	ui "github.com/gizak/termui/v3"
)

type Screen struct {
	*ui.Grid
	popup          ui.Drawable
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
	if s.popup != nil {
		ui.Render(s.popup)
	}
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
		if f, ok := s.focus.(Focusable); ok {
			f.OnFocusOut()
		}
		s.focusStack = append([]Pane{s.focus}, s.focusStack...)
	}
	s.focus = focusable
	if f, ok := s.focus.(Focusable); ok {
		f.OnFocusIn()
	}
}

func (s *Screen) popFocus() bool {
	if len(s.focusStack) == 0 {
		return false
	}
	if f, ok := s.focus.(Focusable); ok {
		f.OnFocusOut()
	}
	s.focus = s.focusStack[0]
	if f, ok := s.focus.(Focusable); ok {
		f.OnFocusIn()
	}
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
		if s.popup != nil {
			s.popFocus()
			s.removePopup()
			return true, false
		}
		if s.focus == s.menu {
			return false, false
		}
		if len(s.rightPaneStack) > 1 {
			s.popRightPane()
			return true, false
		} else {
			return s.popFocus(), false
		}
	case "<F5>", "<C-r>":
		s.reloadCurrentRightPane()
		return false, false
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
			ShowErrorDialog(err, func() error {
				s.popFocus()
				s.popRightPane()
				return nil
			})
		}
		s.popRightPane()
		s.Render()
	}()
}

func (s *Screen) setPopup(p ui.Drawable) {
	s.popup = p
}

func (s *Screen) removePopup() {
	s.popup = nil
}
