package commander

type Screen interface {
	Widget
	SetWorkspace(workspace Workspace)
	Workspace() Workspace
	View() View
	Update()
}

type ErrorHandler interface {
	HandleError(err error)
}
