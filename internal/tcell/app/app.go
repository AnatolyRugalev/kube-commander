package app

import (
	"github.com/AnatolyRugalev/kube-commander/internal/client"
	"github.com/AnatolyRugalev/kube-commander/internal/cmd"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/focus"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/menu"
	"github.com/AnatolyRugalev/kube-commander/internal/tcell/widgets/resources"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type App struct {
	*views.Application
	client  client.Client
	tScreen tcell.Screen
	screen  *widgets.Screen

	namespaceSelector *listTable.ListTable
	selectedNamespace string
}

func New(client client.Client, namespace string) *App {
	app := &App{
		Application:       &views.Application{},
		client:            client,
		selectedNamespace: namespace,
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
	a.tScreen = tScreen

	a.screen = widgets.NewScreen(a, func() string {
		return a.selectedNamespace
	})

	title := views.NewTextBar()
	title.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorTeal).
		Foreground(tcell.ColorWhite))
	title.SetCenter("kube-commander", tcell.StyleDefault)

	a.screen.SetTitle(title)

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
	a.namespaceSelector, err = NewNamespaceSelector(a, a.client, versionResources["Namespace"])
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
		{
			Kind:  "Ingress",
			Title: "Ingresses",
		},
	}, versionResources, func(command string) error {
		return a.SwitchScreen(func() error {
			return cmd.Shell(command)
		})
	}, func() string {
		return a.selectedNamespace
	})
	m := menu.NewMenu(items)

	m.Watch(widgets.NewMenuSelectWatcher(a.screen))

	main := widgets.NewScreenLayout(m, 0.1)
	main.SetStyle(tcell.StyleDefault.
		Background(tcell.ColorBlack))

	a.screen.SetMain(main)
	a.screen.SetKeybar(keybar)

	a.screen.UpdateKeys()
	a.SetRootWidget(a.screen)

	a.screen.SwitchWorkspace(items[0].Widget())
	return nil
}

func (a *App) NamespaceSelector() focus.FocusableWidget {
	return a.namespaceSelector
}

func (a *App) SwitchScreen(switchFunc func() error) error {
	a.tScreen.Clear()
	a.tScreen.Sync()
	err := switchFunc()
	a.tScreen.Sync()
	return err
}
