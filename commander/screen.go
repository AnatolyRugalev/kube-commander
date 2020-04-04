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
	Info(msg string)
}

type ScreenUpdater interface {
	UpdateScreen()
	Resize()
}
