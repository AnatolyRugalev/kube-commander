package tui

import (
	"fmt"
	"image"
	"log"
	"sync"
	"time"

	"github.com/AnatolyRugalev/kube-commander/internal/cmd"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	ui "github.com/gizak/termui/v3"
	"github.com/pkg/errors"
)

type Screen struct {
	*ui.Grid
	menu    *MenuList
	hotkeys *widgets.HotKeysBar

	popupM *sync.Mutex
	popup  Pane

	preloader *widgets.Preloader

	rightPaneStackM *sync.Mutex
	rightPaneStack  []Pane

	focusM     *sync.Mutex
	focusStack []Pane
	focus      Pane

	clickMux      *sync.Mutex
	lastLeftClick time.Time

	handleEvents bool

	selectedNamespace string

	refreshTimer    *time.Timer
	refreshInterval int

	exit chan struct{}
}

func NewScreen() *Screen {
	s := &Screen{
		Grid:            ui.NewGrid(),
		hotkeys:         widgets.NewHotKeysBar(),
		popupM:          &sync.Mutex{},
		rightPaneStackM: &sync.Mutex{},
		focusM:          &sync.Mutex{},
		clickMux:        &sync.Mutex{},
		preloader:       widgets.NewPreloader(),
	}
	s.startAutoRefresh()
	return s
}

func (s *Screen) SwitchToCommand(command string) {
	if Application.Debug {
		log.Printf("Executing command: %s", command)
	}
	s.Switch(func() error {
		return cmd.Shell(command)
	}, func(err error) {
		s.ShowDialog(NewErrorDialog(errors.Wrap(err, fmt.Sprintf("error executing command %s", command)), nil))
	})
}

func (s *Screen) Close() {
	mouseMoveEvents(false)
	ui.Close()
	s.handleEvents = false
}

func (s *Screen) Switch(switchFunc func() error, onError func(error)) {
	s.Close()

	go func() {
		err := switchFunc()
		s.Init()
		if err != nil {
			onError(err)
		}
		s.RenderAll()
	}()
}

func (s *Screen) Render() {
	if s.popup != nil {
		ui.Render(s.popup)
	} else {
		ui.Render(s)
	}
	ui.Render(s.preloader)
}

func (s *Screen) RenderAll() {
	ui.Render(s)
	if s.popup != nil {
		s.centerDialog(s.popup)
		ui.Render(s.popup)
	}
	ui.Render(s.preloader)
}

func (s *Screen) Draw(buf *ui.Buffer) {
	s.setGrid()
	s.Grid.Draw(buf)
}

func (s *Screen) Init() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	termWidth, termHeight := ui.TerminalDimensions()
	s.SetRect(0, 0, termWidth, termHeight)
	s.handleEvents = true
	mouseMoveEvents(true)
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

	menuRatio := 17.0 / float64(s.Rectangle.Max.X)
	hotkeysHeight := 1.0 / float64(s.Rectangle.Max.Y)
	s.Set(
		ui.NewRow(1.0-hotkeysHeight,
			ui.NewCol(menuRatio, s.menu),
			ui.NewCol(1-menuRatio, right),
		),
		ui.NewRow(hotkeysHeight,
			ui.NewCol(1.0, s.hotkeys),
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
	s.updateHotkeys()
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
	s.updateHotkeys()
	return true
}

func (s *Screen) ResetFocus() {
	s.focusStack = []Pane{}
	s.focus = nil
	s.Focus(s.menu)
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

func (s *Screen) updateHotkeys() {
	s.hotkeys.Clear()
	s.hotkeys.SetHotKey(1, "F5", "Refresh")
	var auto string
	interval := refreshIntervals[s.refreshInterval]
	if interval == 0 {
		auto = "Off"
	} else {
		auto = fmt.Sprintf("%ds", int(interval.Seconds()))
	}
	s.hotkeys.SetHotKey(2, "C-B", "Auto:"+auto)
	s.hotkeys.SetHotKey(10, "Q", "Quit")
	if a, ok := s.focus.(widgets.HasHotKeys); ok {
		for i, key := range a.GetHotKeys() {
			s.hotkeys.SetHotKey(i+3, key.Key, key.Name)
		}
	}
}

func (s *Screen) onEvent(event *ui.Event) (bool, bool) {
	switch event.ID {
	case "<Resize>":
		payload := event.Payload.(ui.Resize)
		s.SetRect(0, 0, payload.Width, payload.Height)
		ui.Clear()
		return true, true
	case "<F5>", "<C-r>":
		s.reloadCurrentRightPane()
		return false, false
	case "<C-b>":
		s.toggleAutoRefresh()
		s.updateHotkeys()
		return false, true
	case "<C-n>":
		s.ShowNamespaceSelection()
		return true, false
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
	found, redraw := s.locateAndFocus(m.X, m.Y)
	if found {
		if doubleClick != nil {
			return s.focus.OnEvent(doubleClick), false
		}
		return s.focus.OnEvent(event), false
	}
	return redraw, false
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

func (s *Screen) locateAndFocus(x, y int) (bool, bool) {
	rect := image.Rect(x, y, x+1, y+1)
	if rect.In(screen.focus.Bounds()) {
		return true, true
	}
	if s.popup != nil {
		popupRect := s.popup.GetRect()
		if !rect.In(popupRect.Bounds()) {
			s.popFocus()
			s.removePopup()
			return false, true
		} else {
			return true, true
		}
	} else {
		if rect.In(s.menu.Bounds()) {
			s.popFocus()
			s.Focus(s.menu)
			return true, true
		}

		s.rightPaneStackM.Lock()
		defer s.rightPaneStackM.Unlock()
		if len(s.rightPaneStack) == 0 {
			return false, false
		}
		rightPaneCurrent := s.rightPaneStack[len(s.rightPaneStack)-1]
		if rect.In(rightPaneCurrent.Bounds()) {
			s.popFocus()
			s.Focus(rightPaneCurrent)
			return true, true
		}
	}
	return false, false
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
	if len(s.rightPaneStack) == 0 {
		return
	}
	pane, ok := s.rightPaneStack[0].(Loadable)
	if !ok {
		s.rightPaneStackM.Unlock()
		return
	}
	s.rightPaneStackM.Unlock()

	s.preloader.Run(s.Rectangle, func() error {
		err := pane.Reload()
		return err
	}, func(err error) {
		s.ShowDialog(
			NewLoadingErrorDialog(err, func() error {
				s.popRightPane()
				return nil
			}, func() error {
				s.Exit()
				return nil
			}),
		)
		s.Render()
	})
	s.Render()
}

func (s *Screen) setPopup(p Pane) {
	s.popupM.Lock()
	s.popup = p
	s.popupM.Unlock()
}

func (s *Screen) removePopup() {
	s.popupM.Lock()
	s.popup = nil
	s.popupM.Unlock()
}

func (s *Screen) SetNamespace(namespace string) {
	s.selectedNamespace = namespace
	s.menu.updateMenu(s.selectedNamespace)
}

func (s *Screen) Run() {
	uiEvents := ui.PollEvents()
	s.exit = make(chan struct{})
	for {
		select {
		case e := <-uiEvents:
			if !s.handleEvents {
				continue
			}
			if Application.Debug {
				log.Printf("Handling event: %s", e.ID)
			}
			if e.ID == "<C-c>" || e.ID == "q" {
				return
			}
			redraw, redrawAll := s.onEvent(&e)
			if redrawAll {
				s.RenderAll()
			} else if redraw {
				s.Render()
			}
			if Application.Debug {
				log.Printf("RowEvent handling finished: %s", e.ID)
			}
		case <-s.exit:
			return
		}
	}
}

func (s *Screen) Exit() {
	close(s.exit)
}

func (s *Screen) ShowDialog(dialog Pane) {
	s.centerDialog(dialog)
	s.Focus(dialog)
	s.setPopup(dialog)
}

func (s *Screen) ShowContextMenu(position *image.Point, menu Pane) {
	s.resizeContextMenu(position, menu)
	s.Focus(menu)
	s.setPopup(menu)
}

func (s *Screen) centerDialog(dialog Pane) {
	if r, ok := dialog.(Resizable); ok {
		r.Resize(s.Inner)
	} else {
		rect := dialog.GetRect()
		width := rect.Max.X - rect.Min.X
		height := rect.Max.Y - rect.Min.Y
		if height > s.Inner.Max.Y {
			height = s.Inner.Max.Y
		}
		x := s.Inner.Max.X/2 - width/2
		y := s.Inner.Max.Y/2 - height/2
		dialog.SetRect(x, y, x+width, y+height)
	}
}

func (s *Screen) resizeContextMenu(position *image.Point, dialog Pane) {
	rect := dialog.GetRect()
	width := rect.Max.X - rect.Min.X
	height := rect.Max.Y - rect.Min.Y
	maxHeight := s.Inner.Max.Y
	if height > maxHeight {
		height = maxHeight
	}
	x1 := position.X
	y1 := position.Y
	x2 := x1 + width
	y2 := y1 + height

	if y2 >= s.Inner.Max.Y {
		y1 = s.Inner.Max.Y - height
		y2 = s.Inner.Max.Y
	}
	if x2 >= s.Inner.Max.X {
		x1 = s.Inner.Max.X - width
		x2 = s.Inner.Max.X
	}
	dialog.SetRect(x1, y1, x2, y2)
}
