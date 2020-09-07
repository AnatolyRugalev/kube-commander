package commander

type Workspace interface {
	Widget
	ResourceContainer
	ConfigUpdater
	Init() error
	ShowPopup(title string, widget MaxSizeWidget)
	FocusManager() FocusManager
}

type NamespaceAccessor interface {
	CurrentNamespace() string
}
