package commander

type Screen interface {
	ScreenUpdater
	Widget
	SetWorkspace(workspace Workspace)
	SetStatus(status StatusReporter)
	Workspace() Workspace
	View() View
}

type StatusReporter interface {
	Widget
	Error(err error)
	Warning(msg string)
	Info(msg string)
	Confirm(msg string) bool
}

type ScreenUpdater interface {
	UpdateScreen()
	Resize()
}
