package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Screen struct {
	views.Panel
	handler   AppHandler
	main      *ScreenLayout
	keybar    *views.SimpleStyledText
	namespace listTable.NamespaceAccessor
}

func NewScreen(handler AppHandler, namespace listTable.NamespaceAccessor) *Screen {
	return &Screen{
		handler:   handler,
		namespace: namespace,
	}
}

func (s *Screen) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyCtrlL:
			s.handler.Refresh()
			return true
		case tcell.KeyCtrlN:
			s.main.PostEvent(focus.NewPopupEvent(
				s.handler.NamespaceSelector(),
				0.5,
				0.5,
			))
			return true
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'Q', 'q':
				s.handler.Quit()
				return true
			}
		}
	}
	return s.Panel.HandleEvent(ev)
}

func (s *Screen) Draw() {
	s.UpdateKeys()
	s.Panel.Draw()
}

func (s *Screen) Blur() {
	s.main.focus.Blur()
}

func (s *Screen) UpdateKeys() {
	w := "[%AQ%N] Quit"
	ns := s.namespace()
	if ns == "" {
		ns = client.AllNamespaces
	}
	w += "  Namespace: " + ns
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

type AppHandler interface {
	Update()
	Refresh()
	Quit()
	NamespaceSelector() focus.FocusableWidget
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
