package app

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui"
	"github.com/AnatolyRugalev/kube-commander/app/ui/workspace"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type app struct {
	tApp    *views.Application
	tScreen tcell.Screen

	client           commander.Client
	resourceProvider commander.ResourceProvider
	commandBuilder   commander.CommandBuilder
	commandExecutor  commander.CommandExecutor
	screen           commander.Screen
	workspace        commander.Workspace

	defaultNamespace string

	quit chan struct{}
}

func (a app) Quit() {
	close(a.quit)
}

func NewApp(client commander.Client, resourceProvider commander.ResourceProvider, commandBuilder commander.CommandBuilder, commandExecutor commander.CommandExecutor, defaultNamespace string) *app {
	a := app{
		client:           client,
		resourceProvider: resourceProvider,
		commandBuilder:   commandBuilder,
		commandExecutor:  commandExecutor,
		defaultNamespace: defaultNamespace,

		quit: make(chan struct{}),
	}
	a.commandExecutor = NewAppExecutor(&a, commandExecutor)
	return &a
}

func (a app) Client() commander.Client {
	return a.client
}

func (a app) ResourceProvider() commander.ResourceProvider {
	return a.resourceProvider
}

func (a app) CommandBuilder() commander.CommandBuilder {
	return a.commandBuilder
}

func (a app) CommandExecutor() commander.CommandExecutor {
	return a.commandExecutor
}

func (a app) Screen() commander.Screen {
	return a.screen
}

func (a app) Update() {
	a.tApp.Update()
}

func (a app) CurrentNamespace() string {
	return a.defaultNamespace
}

func (a app) ErrorHandler() commander.ErrorHandler {
	return a.screen.Workspace()
}

func (a *app) initScreen() (err error) {
	a.tApp = &views.Application{}
	a.tScreen, err = tcell.NewScreen()
	if err != nil {
		return
	}
	a.tApp.SetScreen(a.tScreen)
	a.tApp.SetRootWidget(a.screen)
	return
}

func (a *app) Interrupt(f func() error) error {
	a.tApp.Quit()
	err := a.tApp.Wait()
	if err != nil {
		return err
	}
	err = f()
	if err := a.initScreen(); err != nil {
		panic(err)
	}
	a.tApp.Start()
	return err
}

func (a *app) Run() error {
	a.workspace = workspace.NewWorkspace(a, a.defaultNamespace)
	err := a.workspace.Init()
	if err != nil {
		return err
	}
	a.screen = ui.NewScreen(a)
	a.screen.SetWorkspace(a.workspace)

	err = a.initScreen()
	if err != nil {
		return err
	}
	a.tApp.Start()

	<-a.quit

	a.tApp.Quit()
	return a.tApp.Wait()
}
