package app

import (
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/AnatolyRugalev/kube-commander/internal/cmd"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/resources"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type App struct {
	*views.Application
	client client.Client
	screen tcell.Screen
}

func New(client client.Client) *App {
	app := &App{
		Application: &views.Application{},
		client:      client,
	}
	app.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack))
	return app
}

func (a *App) InitScreen() error {
	tScreen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	a.SetScreen(tScreen)
	a.screen = tScreen

	screen := widgets.NewScreen(a)

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	title.SetCenter("kube-commander", tcell.StyleDefault)

	screen.SetTitle(title)

	keybar := views.NewSimpleStyledText()
	keybar.RegisterStyle('N', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorBlack))
	keybar.RegisterStyle('A', tcell.StyleDefault.
		Background(tcell.ColorSilver).
		Foreground(tcell.ColorRed))

	versionResources, err := a.client.PreferredGroupVersionResources()
	if err != nil {
		return err
	}
	items := resources.BuildResourceMenu(a.client, []resources.ResourceItem{
		{
			Kind:  "Namespace",
			Title: "Namespaces",
		},
		{
			Kind:  "Node",
			Title: "Nodes",
		},
		{
			Kind:  "Pod",
			Title: "Pods",
		},
		{
			Kind:  "Ingress",
			Title: "Ingresses",
		},
		{
			Kind:  "Deployment",
			Title: "Deployments",
		},
	}, versionResources, func(command string) error {
		return a.SwitchScreen(func() error {
			return cmd.Shell(command)
		})
	}, func() string {
		//TODO: implement
		return ""
	})
	m := menu.NewMenu(items)

	m.Watch(widgets.NewMenuSelectWatcher(screen))

	main := widgets.NewScreenLayout(m, 0.1)
	main.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack))

	screen.SetMain(main)
	screen.SetKeybar(keybar)

	screen.UpdateKeys()
	a.SetRootWidget(screen)

	screen.SwitchWorkspace(items[0].Widget())
	return nil
}

func (a *App) SwitchScreen(switchFunc func() error) error {
	a.screen.Clear()
	a.screen.Sync()
	err := switchFunc()
	a.screen.Sync()
	return err
}
