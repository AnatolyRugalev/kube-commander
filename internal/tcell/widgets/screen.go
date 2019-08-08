package widgets

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type screenModel struct {
	x    int
	y    int
	endx int
	endy int
	hide bool
	enab bool
	loc  string
}

func (s *screenModel) GetCell(x, y int) (rune, tcell.Style, []rune, int) {
	dig := []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
	var ch rune
	style := tcell.StyleDefault
	if x >= 60 || y >= 15 {
		return ch, style, nil, 1
	}
	colors := []tcell.Color{
		tcell.ColorWhite,
		tcell.ColorGreen,
		tcell.ColorMaroon,
		tcell.ColorNavy,
		tcell.ColorOlive,
	}
	if y == 0 && x < len(s.loc) {
		style = style.
			Foreground(tcell.ColorWhite).
			Background(tcell.ColorLime)
		ch = rune(s.loc[x])
	} else {
		ch = dig[(x)%len(dig)]
		style = style.
			Foreground(colors[(y)%len(colors)]).
			Background(tcell.ColorBlack)
	}
	return ch, style, nil, 1
}

func (s *screenModel) GetBounds() (int, int) {
	return s.endx, s.endy
}
func (s *screenModel) MoveCursor(offx, offy int) {
	s.x += offx
	s.y += offy
	s.limitCursor()
}

func (s *screenModel) limitCursor() {
	if s.x < 0 {
		s.x = 0
	}
	if s.x > s.endx-1 {
		s.x = s.endx - 1
	}
	if s.y < 0 {
		s.y = 0
	}
	if s.y > s.endy-1 {
		s.y = s.endy - 1
	}
	s.loc = fmt.Sprintf("Cursor is %d,%d", s.x, s.y)
}

func (s *screenModel) GetCursor() (int, int, bool, bool) {
	return s.x, s.y, s.enab, !s.hide
}

func (s *screenModel) SetCursor(x int, y int) {
	s.x = x
	s.y = y

	s.limitCursor()
}

type Screen struct {
	views.Panel
	handler ScreenHandler
	main    *ScreenLayout
	keybar  *views.SimpleStyledText
	status  *views.SimpleStyledTextBar
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
				s.updateKeys()
				return true
			case 'H', 'h':
				s.model.hide = true
				s.updateKeys()
				return true
			case 'E', 'e':
				s.model.enab = true
				s.updateKeys()
				return true
			case 'D', 'd':
				s.model.enab = false
				s.updateKeys()
				return true
			}
		}
	}
	return s.Panel.HandleEvent(ev)
}

func (s *Screen) Draw() {
	s.status.SetLeft(s.model.loc)
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
	s.handler.Update()
}

type ScreenHandler interface {
	Update()
	Refresh()
	Quit()
}

type MenuEventHandler struct {
	focusOnEnter focus.FocusableWidget
}

func (m MenuEventHandler) HandleListEvent(event *listTable.Event) bool {
	switch ev := event.Event.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEnter:
			event.ListTable.PostEvent(focus.NewFocusEvent(event.ListTable, m.focusOnEnter))
			return true
		}
	}
	return false
}

func NewScreen(handler ScreenHandler) *Screen {
	screen := &Screen{
		handler: handler,
	}
	screen.model = &screenModel{
		endx: 60,
		endy: 15,
	}

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	title.SetCenter("CellView Test", tcell.StyleDefault)
	title.SetRight("Example v1.0", tcell.StyleDefault)

	screen.keybar = views.NewSimpleStyledText()
	screen.keybar.RegisterStyle('N', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorBlack))
	screen.keybar.RegisterStyle('A', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorRed))

	screen.status = views.NewSimpleStyledTextBar()
	screen.status.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlue).
		Foreground(tcell.ColorYellow))
	screen.status.RegisterLeftStyle('N', tcell.StyleDefault.
		Background(tcell.ColorYellow).
		Foreground(tcell.ColorBlack))

	screen.status.SetLeft("My status is here.")
	screen.status.SetRight("%UCellView%N demo!")
	screen.status.SetCenter("Cen%ST%Ner")

	rows := []listTable.Row{
		{"Test", "test", "test333333"},
		{"Test 2", "teestestset", "tesete412321"},
		{"Test 3", "teestestset", "tesete412321"},
		{"Test 4", "teestestset", "tesete412321"},
		{"Test 5", "teestestset", "tesete412321"},
		{"Test 6", "teestestset", "tesete412321"},
		{"Test 7", "teestestset", "vbu32oi7ugvewiuvy32ugviueyfge2vuiyekjhcebuyvwejvhguweyvjqwghcvuyejgb23uvyejhb2vuyejgcb32uciv32uib32yhcv32uycv32yc3vbveucy32jgcv23uycjg32bcvty32chg3bvuyc23bcvuy23cvb3iyu23vc32iuyvcu32ycv23utcfv32biucvyu3biu32yvcui32cv32uycb32icv32yuc"},
		{"Test 8", "teestestset", "tesete412321"},
		{"Test 9", "teestestset", "tesete412321"},
		{"Test 11", "teestestset", "tesete412321"},
		{"Test 12", "teestestset", "tesete412321"},
		{"Test 13", "teestestset", "tesete412321"},
		{"Test 14", "teestestset", "tesete412321"},
		{"Test 15", "teestestset", "tesete412321"},
		{"Test 16", "teestestset", "tesete412321"},
		{"Test 17", "teestestset", "tesete412321"},
		{"Test 18", "teestestset", "tesete412321"},
		{"Test 19", "teestestset", "tesete412321"},
		{"Test 20", "teestestset", "tesete412321"},
	}
	workspace := listTable.NewListTable([]listTable.Column{
		listTable.NewStringColumn("column 1"),
		listTable.NewStringColumn("C2"),
		listTable.NewStringColumn("Col 3"),
	}, rows, true)

	menu := listTable.NewList([]string{"Nodes", "Namespaces", "Pods"})
	menu.SetEventHandler(&MenuEventHandler{focusOnEnter: workspace})

	screen.main = NewScreenLayout(menu, 0.25)
	screen.main.AddWidget(workspace, 0.75)
	screen.main.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack))

	screen.SetMenu(screen.keybar)
	screen.SetTitle(title)
	screen.SetContent(screen.main)
	screen.SetStatus(screen.status)

	screen.updateKeys()

	return screen
}
