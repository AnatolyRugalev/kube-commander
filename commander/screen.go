package commander

type Screen interface {
	Widget
	ScreenUpdater
	SetWorkspace(workspace Workspace)
	Workspace() Workspace
	View() View
}

type ErrorHandler interface {
	HandleError(err error)
}

type ScreenUpdater interface {
	UpdateScreen()
	Resize()
}
