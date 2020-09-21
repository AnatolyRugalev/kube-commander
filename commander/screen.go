package commander

type Screen interface {
	ScreenHandler
	Widget
	Init(status StatusReporter, theme ThemeManager)
	SetWorkspace(workspace Workspace)
	Workspace() Workspace
	View() View
}

type StatusReporter interface {
	Widget
	Error(err error)
	Warning(msg string)
	Info(msg string)
	Confirm(msg string) bool
	LoadingStarted()
	LoadingFinished()
}

type ScreenHandler interface {
	Status() StatusReporter
	UpdateScreen()
	Resize()
	Theme() ThemeManager
}
