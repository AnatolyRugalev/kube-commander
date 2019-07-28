package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type App struct {
	*views.Application
}

func NewApp() *App {
	app := &App{
		&views.Application{},
	}
	app.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack))
	screen := widgets.NewScreen(app)
	app.SetRootWidget(screen)
	return app
}
