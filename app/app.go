package app

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app/ui"
	"github.com/AnatolyRugalev/kube-commander/app/ui/status"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/app/ui/workspace"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/AnatolyRugalev/kube-commander/config"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"sync"
)

type app struct {
	tApp    *views.Application
	tScreen tcell.Screen

	config           commander.Config
	client           commander.Client
	resourceProvider commander.ResourceProvider
	commandBuilder   commander.CommandBuilder
	commandExecutor  commander.CommandExecutor
	screen           commander.Screen
	workspace        commander.Workspace
	status           commander.StatusReporter

	defaultNamespace string

	configCh      chan config.Event
	configMu      sync.Mutex
	configurables []commander.Configurable
	quit          chan struct{}
	configPath    string
}

func (a *app) Quit() {
	close(a.quit)
}

func NewApp(config commander.Config, client commander.Client, resourceProvider commander.ResourceProvider, commandBuilder commander.CommandBuilder, commandExecutor commander.CommandExecutor, defaultNamespace string, configCh chan config.Event, configPath string) *app {
	a := app{
		config:           config,
		client:           client,
		resourceProvider: resourceProvider,
		commandBuilder:   commandBuilder,
		commandExecutor:  commandExecutor,
		defaultNamespace: defaultNamespace,

		configCh:   configCh,
		configPath: configPath,
		quit:       make(chan struct{}),
	}
	a.commandExecutor = NewAppExecutor(&a, commandExecutor)
	return &a
}

func (a *app) Config() commander.Config {
	return a.config
}

func (a *app) Client() commander.Client {
	return a.client
}

func (a *app) ResourceProvider() commander.ResourceProvider {
	return a.resourceProvider
}

func (a *app) CommandBuilder() commander.CommandBuilder {
	return a.commandBuilder
}

func (a *app) CommandExecutor() commander.CommandExecutor {
	return a.commandExecutor
}

func (a *app) Screen() commander.Screen {
	return a.screen
}

func (a *app) Update() {
	a.tApp.Update()
}

func (a *app) CurrentNamespace() string {
	return a.defaultNamespace
}

func (a *app) StatusReporter() commander.StatusReporter {
	return a.status
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
	a.screen = ui.NewScreen(a)
	err := a.initScreen()
	if err != nil {
		return fmt.Errorf("could not initialize screen: %w", err)
	}
	a.workspace = workspace.NewWorkspace(a, a.defaultNamespace)
	a.screen.SetWorkspace(a.workspace)

	a.status = status.NewStatus(a.screen)
	themeManager := theme.NewManager(a.screen, a.status, a)
	a.Register(themeManager)

	a.screen.Init(a.status, themeManager)
	err = a.workspace.Init()
	if err != nil {
		return err
	}
	a.tApp.Start()

	go a.watchConfig()

	<-a.quit

	a.tApp.Quit()
	return a.tApp.Wait()
}

func (a *app) watchConfig() {
	for event := range a.configCh {
		if event.Err != nil {
			a.status.Error(fmt.Errorf("config: %w", event.Err))
			continue
		}
		for _, c := range a.configurables {
			c.ConfigUpdated(event.Config)
		}
	}
}

func (a *app) Register(c commander.Configurable) {
	a.configurables = append(a.configurables, c)
}

func (a *app) ConfigUpdater() commander.ConfigUpdater {
	return a
}

func (a *app) UpdateConfig(f commander.ConfigUpdateFunc) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	f(cfg)
	err = config.Save(a.configPath, cfg)
	if err != nil {
		return err
	}
	return nil
}
