package ui

import (
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type Screen struct {
	*views.Panel
	focus.Focusable

	app       commander.App
	workspace commander.Workspace
	view      commander.View
}

func (s *Screen) View() commander.View {
	return s.view
}

func (s *Screen) SetView(view views.View) {
	s.view = view
	s.Panel.SetView(view)
}

func (s *Screen) UpdateScreen() {
	s.app.Update()
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
		Panel: views.NewPanel(),
		app:   app,
	}

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	title.SetCenter("kube-commander", tcell.StyleDefault)

	s.SetTitle(title)

	return &s
}

func (s Screen) HandleError(err error) {
	panic(err)
}

func (s Screen) HandleEvent(e tcell.Event) bool {
	switch ev := e.(type) {
	case *tcell.EventKey:
		if ev.Rune() == 'q' && ev.Modifiers() == tcell.ModNone {
			s.app.Quit()
			return true
		}
	}
	return s.BoxLayout.HandleEvent(e)
}
