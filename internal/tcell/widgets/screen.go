package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/resources"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/events"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Screen struct {
	views.Panel
	handler ScreenHandler
	main    *ScreenLayout
	keybar  *views.SimpleStyledText
	model   *screenModel
}

func NewScreen(handler ScreenHandler) *Screen {
	return &Screen{
		handler: handler,
	}
}

func (s *Screen) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyCtrlL:
			s.handler.Refresh()
			return true
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'Q', 'q':
				s.handler.Quit()
				return true
			case 'N', 'n':
				nsMenu := resources.NewNamespacesMenu()
				nsMenu.SetEventHandler(&nsMenuHandler{
					screen: s,
				})
				s.main.PostEvent(focus.NewPopupEvent(
					nsMenu,
					0.5,
					0.5,
				))
				return true
			}
		}
	}
	return s.Panel.HandleEvent(ev)
}

type nsMenuHandler struct {
	screen *Screen
}

func (n nsMenuHandler) HandleRowEvent(event listTable.RowEvent) bool {
	switch ev := event.(type) {
	case *listTable.RowTcellEvent:
		switch ev := ev.TcellEvent().(type) {
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEnter:
				row := event.Row()
				if row == nil {
					return false
				}
				n.screen.model.namespace = row[0].(string)
				n.screen.main.focus.Blur()
				n.screen.PostEvent(events.NewNamespaceChanged(event.ListTable(), row[0].(string)))
				return true
			}
		}
	}
	return false
}

func (s *Screen) Draw() {
	s.UpdateKeys()
	s.Panel.Draw()
}

func (s *Screen) UpdateKeys() {
	w := "[%AQ%N] Quit"
	w += "  Namespace: " + m.namespace
	s.keybar.SetMarkup(w)
}

func (s *Screen) SetKeybar(bar *views.SimpleStyledText) {
	s.keybar = bar
	s.SetStatus(s.keybar)
}

func (s *Screen) SetMain(main *ScreenLayout) {
	s.main = main
	s.SetContent(s.main)
}

type ScreenHandler interface {
	Update()
	Refresh()
	Quit()
}

type DisplayableWidget interface {
	OnDisplay()
}

func (s *Screen) SwitchWorkspace(widget views.Widget) {
	widgets := s.main.Widgets()
	if len(widgets) == 2 {
		s.main.RemoveWidget(widgets[len(widgets)-1])
	}
	s.main.AddWidget(widget, 0.9)
	if w, ok := widget.(DisplayableWidget); ok {
		w.OnDisplay()
	}
}

func NewMenuSelectWatcher(screen *Screen) *MenuSelectWatcher {
	return &MenuSelectWatcher{
		screen: screen,
	}
}

type MenuSelectWatcher struct {
	screen *Screen
}

func (m MenuSelectWatcher) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *menu.SelectEvent:
		m.screen.main.SwitchWorkspace(ev.Widget())
		return true
	}
	return false
}
