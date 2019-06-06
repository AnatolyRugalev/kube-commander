package tui

import (
	"fmt"
	"image"
	"log"
	"sync"
	"time"

	"github.com/AnatolyRugalev/kube-commander/internal/cmd"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	ui "github.com/gizak/termui/v3"
	"github.com/pkg/errors"
)

type Screen struct {
	*ui.Grid
	menu *widgets.ListTable

	popupM *sync.Mutex
	popup  ui.Drawable

	rightPaneStackM *sync.Mutex
	rightPaneStack  []Pane

	focusM     *sync.Mutex
	focusStack []Pane
	focus      Pane

	clickMux      *sync.Mutex
	lastLeftClick time.Time

	handleEvents bool
}

func NewScreen() *Screen {
	s := &Screen{
		Grid:            ui.NewGrid(),
		popupM:          &sync.Mutex{},
		rightPaneStackM: &sync.Mutex{},
		focusM:          &sync.Mutex{},
		clickMux:        &sync.Mutex{},
	}
	return s
}

func (s *Screen) SwitchToCommand(command string) {
	s.Switch(func() error {
		return cmd.Shell(command)
	}, func(err error) {
		ShowErrorDialog(errors.Wrap(err, fmt.Sprintf("error executing command %s", command)), nil)
	})
}

func (s *Screen) Switch(switchFunc func() error, onError func(error)) {
	mouseMoveEvents(false)
	ui.Close()
	s.handleEvents = false
	go func() {
		err := switchFunc()
		if err := ui.Init(); err != nil {
			log.Fatalf("failed to initialize termui: %v", err)
		}
		s.Init()
		mouseMoveEvents(true)
		if err != nil {
			onError(err)
		}
		s.Render()
	}()
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
	s.handleEvents = true
}

func (s *Screen) SetMenu(menu *widgets.ListTable) {
	s.menu = menu
}

func (s *Screen) setGrid() {
	s.rightPaneStackM.Lock()
	s.Items = []*ui.GridItem{}
	var right interface{}
	if len(s.rightPaneStack) > 0 {
		right = s.rightPaneStack[0]
	}

	menuRatio := 17.0 / float64(s.Rectangle.Max.X)
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

func (s *Screen) onEvent(event *ui.Event) (bool, bool) {
	switch event.ID {
	case "q", "<C-c>":
		return false, true
	case "<Resize>":
		payload := event.Payload.(ui.Resize)
		s.SetRect(0, 0, payload.Width, payload.Height)
		ui.Clear()
		return true, false
	case "<F5>", "<C-r>":
		s.reloadCurrentRightPane()
		return false, false
	case "<MouseLeft>":
		return s.mouseLeftEvent(event)
	}

	var focusReaction bool
	if s.focus != nil {
		focusReaction = s.focus.OnEvent(event)
	}
	if !focusReaction && event.ID == "<Escape>" {
		return s.escape(), false
	}
	return focusReaction, false
}

func (s *Screen) mouseLeftEvent(event *ui.Event) (bool, bool) {
	var doubleClick *ui.Event
	s.clickMux.Lock()
	if time.Since(s.lastLeftClick) <= time.Millisecond*doubleClickSensitive {
		doubleClick = cloneEvent(event, eventMouseLeftDouble)
	}
	s.lastLeftClick = time.Now()
	s.clickMux.Unlock()
	m := event.Payload.(ui.Mouse)
	if s.locateAndFocus(m.X, m.Y) {
		if doubleClick != nil {
			return s.focus.OnEvent(doubleClick), false
		}
		return s.focus.OnEvent(event), false
	}
	return false, false
}

func (s *Screen) escape() bool {
	if s.popup != nil {
		s.popFocus()
		s.removePopup()
		return true
	}
	if s.focus == s.menu {
		return false
	}
	if len(s.rightPaneStack) > 1 {
		s.popRightPane()
		return true
	} else {
		return s.popFocus()
	}
}

func (s *Screen) setRightPane(pane Pane) {
	s.rightPaneStackM.Lock()
	s.rightPaneStack = []Pane{pane}
	s.rightPaneStackM.Unlock()
}

func (s *Screen) locateAndFocus(x, y int) bool {
	rect := image.Rect(x, y, x+1, y+1)
	if rect.In(screen.focus.Bounds()) {
		return true
	}
	if s.popup == nil {
		if rect.In(s.menu.Bounds()) {
			//
			s.popFocus()
			s.Focus(s.menu)
			return true
		}

		s.rightPaneStackM.Lock()
		defer s.rightPaneStackM.Unlock()
		if len(s.rightPaneStack) == 0 {
			return false
		}
		rightPaneCurrent := s.rightPaneStack[len(s.rightPaneStack)-1]
		if rect.In(rightPaneCurrent.Bounds()) {
			s.popFocus()
			s.Focus(rightPaneCurrent)
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
	preloader := widgets.NewPreloader(s.Rectangle, func() error {
		return pane.Reload()
	}, func() {
		s.popFocus()
		s.removePopup()
		s.Render()
	}, func(err error) {
		ShowErrorDialog(err, func() error {
			s.popFocus()
			s.popRightPane()
			return nil
		})
		s.Render()
	}, func() {
		s.removePopup()
		s.popRightPane()
		s.Render()
	})
	s.setPopup(preloader)
	s.Focus(preloader)
	s.Render()
	preloader.Run()
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

func (s *Screen) Edit(resType string, name string, namespace string) {
	if namespace == "" {
		screen.SwitchToCommand(kube.Edit(resType, name))
	} else {
		screen.SwitchToCommand(kube.EditNs(namespace, resType, name))
	}
}

func (s *Screen) Describe(resType string, name string, namespace string) {
	if namespace == "" {
		screen.SwitchToCommand(kube.Viewer(kube.Describe(resType, name)))
	} else {
		screen.SwitchToCommand(kube.Viewer(kube.DescribeNs(namespace, resType, name)))
	}
}

func (s *Screen) Run() {
	uiEvents := ui.PollEvents()
	for {
		select {
		case e := <-uiEvents:
			if !s.handleEvents {
				continue
			}
			redraw, exit := s.onEvent(&e)
			if exit {
				return
			}
			if redraw {
				s.Render()
			}
		}
	}
}
