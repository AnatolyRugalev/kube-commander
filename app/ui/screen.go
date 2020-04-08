package ui

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
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
}

func (s *Screen) View() commander.View {
	return s.view
}

func (s *Screen) SetView(view views.View) {
	s.view = view
	s.Panel.SetView(view)
}

func (s *Screen) SetStatus(stat commander.StatusReporter) {
	s.status = stat
	s.Panel.SetStatus(s.status)

	s.theme = theme.NewManager(s.workspace.FocusManager(), s, s.status)
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

func (s Screen) Workspace() commander.Workspace {
	return s.workspace
}

func NewScreen(app commander.App) *Screen {
	s := Screen{
		Panel:     views.NewPanel(),
		Focusable: focus.NewFocusable(),
		app:       app,
	}

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	title.SetCenter("kube-commander", theme.Default)
	title.SetRight(app.Config().Context(), theme.Default)

	s.SetTitle(title)

	return &s
}

func (s Screen) HandleEvent(e tcell.Event) bool {
	if s.theme.HandleEvent(e) {
		return true
	}
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
		case tcell.KeyF10:
			err := s.theme.Init()
			if err != nil {
				s.status.Error(err)
			}
		case tcell.KeyF11:
			s.theme.DeInit()
		}
	}
	return false
}
