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

type screenModel struct {
	hide      bool
	enab      bool
	namespace string
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
	w += "  Namespace: " + m.namespace
	s.keybar.SetMarkup(w)
}

type ScreenHandler interface {
	Update()
	Refresh()
	Quit()
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
		menu.NewItem("Pods", resources.NewPodsListTable()),
	})

	m.Watch(&MenuSelectWatcher{screen: screen})

	screen.main = NewScreenLayout(m, 0.1)
	screen.main.SwitchWorkspace(namespaces)
	screen.main.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack))

	screen.SetTitle(title)
	screen.SetContent(screen.main)
	screen.SetStatus(screen.keybar)

	screen.updateKeys()

	return screen
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
