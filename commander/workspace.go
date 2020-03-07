package commander

type Workspace interface {
	Widget
	ResourceContainer
	Init() error
	ShowPopup(widget MaxSizeWidget)
	FocusManager() FocusManager
}

type NamespaceAccessor interface {
	CurrentNamespace() string
}
