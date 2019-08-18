package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/resources"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type screenModel struct {
	hide bool
	enab bool
}

type Screen struct {
	views.Panel
	handler ScreenHandler
	main    *ScreenLayout
	keybar  *views.SimpleStyledText
	model   *screenModel
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
			case 'S', 's':
				s.model.hide = false
				return true
			case 'H', 'h':
				s.model.hide = true
				return true
			case 'E', 'e':
				s.model.enab = true
				return true
			case 'D', 'd':
				s.model.enab = false
				return true
			}
		}
	}
	return s.Panel.HandleEvent(ev)
}

func (s *Screen) Draw() {
	s.updateKeys()
	s.Panel.Draw()
}

func (s *Screen) updateKeys() {
	m := s.model
	w := "[%AQ%N] Quit"
	if !m.enab {
		w += "  [%AE%N] Enable cursor"
	} else {
		w += "  [%AD%N] Disable cursor"
		if !m.hide {
			w += "  [%AH%N] Hide cursor"
		} else {
			w += "  [%AS%N] Show cursor"
		}
	}
	s.keybar.SetMarkup(w)
}

type ScreenHandler interface {
	Update()
	Refresh()
	Quit()
}

type DisplayableWidget interface {
	OnDisplay()
}

func NewScreen(handler ScreenHandler) *Screen {
	screen := &Screen{
		handler: handler,
	}
	screen.model = &screenModel{}

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	_ = kube.InitClient()
	title.SetCenter("kube-commander", tcell.StyleDefault)
	title.SetRight(kube.Context(), tcell.StyleDefault)

	screen.keybar = views.NewSimpleStyledText()
	screen.keybar.RegisterStyle('N', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorBlack))
	screen.keybar.RegisterStyle('A', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorRed))

	namespaces := resources.NewNamespacesListTable()

	m := menu.NewMenu([]menu.Item{
		menu.NewItem("Namespaces", namespaces),
		menu.NewItem("Nodes", resources.NewNodesListTable()),
	})

	m.Watch(&MenuSelectWatcher{screen: screen})

	screen.main = NewScreenLayout(m, 0.1)
	screen.SwitchWorkspace(namespaces)
	screen.main.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack))

	screen.SetTitle(title)
	screen.SetContent(screen.main)
	screen.SetStatus(screen.keybar)

	screen.updateKeys()

	return screen
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

type MenuSelectWatcher struct {
	screen *Screen
}

func (m MenuSelectWatcher) HandleEvent(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *menu.SelectEvent:
		m.screen.SwitchWorkspace(ev.Widget())
		return true
	}
	return false
}
