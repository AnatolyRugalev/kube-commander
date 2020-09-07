package ui

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/logo"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Screen struct {
	*views.Panel
	*focus.Focusable

	app       commander.App
	workspace commander.Workspace
	status    commander.StatusReporter
	view      commander.View
	theme     commander.ThemeManager
	title     *views.BoxLayout
	titleBar  *views.TextBar
}

func (s *Screen) View() commander.View {
	return s.view
}

func (s *Screen) SetView(view views.View) {
	s.view = view
	s.Panel.SetView(view)
}

func (s *Screen) Init(status commander.StatusReporter, theme commander.ThemeManager) {
	s.status = status
	s.Panel.SetStatus(s.status)
	s.theme = theme
	s.title.AddWidget(logo.NewLogo(s.theme), 0)
	s.title.AddWidget(s.titleBar, 1.0)
	s.SetTitle(s.title)
}

func (s *Screen) UpdateScreen() {
	if s.app != nil {
		s.app.Update()
	}
}

func (s *Screen) SetWorkspace(workspace commander.Workspace) {
	s.workspace = workspace
	s.SetContent(s.workspace)
}

func (s *Screen) Workspace() commander.Workspace {
	return s.workspace
}

func NewScreen(app commander.App) *Screen {
	s := Screen{
		Panel:     views.NewPanel(),
		Focusable: focus.NewFocusable(),
		app:       app,
		title:     views.NewBoxLayout(views.Horizontal),
		titleBar:  views.NewTextBar(),
	}
	return &s
}

func (s *Screen) Draw() {
	s.titleBar.SetStyle(s.theme.GetStyle("title-bar"))
	s.Panel.SetStyle(s.theme.GetStyle("screen"))
	s.Panel.Draw()
}

func (s *Screen) Theme() commander.ThemeManager {
	return s.theme
}

func (s *Screen) HandleEvent(e tcell.Event) bool {
	if s.BoxLayout.HandleEvent(e) {
		return true
	}
	switch ev := e.(type) {
	case *tcell.EventKey:
		if ev.Rune() == 'q' && ev.Modifiers() == tcell.ModNone {
			s.app.Quit()
			return true
		}
		switch ev.Key() {
		case tcell.KeyF11:
			s.theme.NextTheme()
			return true
		case tcell.KeyF10:
			s.theme.PrevTheme()
			return true
		}
	}
	return false
}
