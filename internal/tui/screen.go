package tui

import (
	"image"
	"sync"

	ui "github.com/gizak/termui/v3"
)

type Screen struct {
	*ui.Grid
	menu *MenuList

	popupM *sync.Mutex
	popup  ui.Drawable

	rightPaneStackM *sync.Mutex
	rightPaneStack  []Pane

	focusM     *sync.Mutex
	focusStack []Pane
	focus      Pane
}

func NewScreen() *Screen {
	s := &Screen{
		Grid:            ui.NewGrid(),
		popupM:          &sync.Mutex{},
		rightPaneStackM: &sync.Mutex{},
		focusM:          &sync.Mutex{},
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
	s.rightPaneStackM.Lock()
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
	s.rightPaneStackM.Unlock()
}

func (s *Screen) Focus(focusable Pane) {
	s.focusM.Lock()
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
	s.focusM.Unlock()
}

func (s *Screen) popFocus() bool {
	s.focusM.Lock()
	defer s.focusM.Unlock()
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
	s.rightPaneStackM.Lock()
	defer s.rightPaneStackM.Unlock()
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
	case "<MouseLeft>":
		m := event.Payload.(ui.Mouse)
		if s.foundAndClick(m.X, m.Y) {
			return s.focus.OnEvent(event), false
		}
		return false, false
	default:
		if s.focus != nil {
			return s.focus.OnEvent(event), false
		}
		return false, false
	}
}

func (s *Screen) setRightPane(pane Pane) {
	s.rightPaneStackM.Lock()
	s.rightPaneStack = []Pane{pane}
	s.rightPaneStackM.Unlock()
}

func (s *Screen) foundAndClick(x, y int) bool {
	rect := image.Rect(x, y, x+1, y+1)
	if rect.In(screen.focus.Bounds()) {
		return true
	}
	if rect.In(s.menu.Bounds()) {
		s.popFocus()
		s.Focus(s.menu)
		return true
	}
	for _, item := range s.rightPaneStack {
		if rect.In(item.Bounds()) {
			s.popFocus()
			s.Focus(item)
			return true
		}
	}
	return false
}

func (s *Screen) appendRightPane(pane Pane) {
	s.rightPaneStackM.Lock()
	refocus := s.focus == s.rightPaneStack[0]
	s.rightPaneStack = append([]Pane{pane}, s.rightPaneStack...)
	if refocus {
		s.Focus(s.rightPaneStack[0])
	}
	s.rightPaneStackM.Unlock()
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
	s.rightPaneStackM.Lock()
	pane, ok := s.rightPaneStack[0].(Loadable)
	if !ok {
		s.rightPaneStackM.Unlock()
		return
	}
	s.rightPaneStackM.Unlock()

	// Add preloader overlay
	// TODO: fix edge-cases
	done := make(chan struct{})
	preloader := NewPreloader(s.Rectangle, done)
	s.setPopup(preloader)
	s.Focus(preloader)
	s.Render()
	go func() {
		err := pane.Reload()
		s.popFocus()
		s.removePopup()
		close(done)
		if err != nil {
			ShowErrorDialog(err, func() error {
				s.popFocus()
				s.popRightPane()
				return nil
			})
		}
		s.Render()
	}()
}

func (s *Screen) setPopup(p ui.Drawable) {
	s.popupM.Lock()
	s.popup = p
	s.popupM.Unlock()
}

func (s *Screen) removePopup() {
	s.popupM.Lock()
	s.popup = nil
	s.popupM.Unlock()
}
