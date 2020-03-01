package commander

type Workspace interface {
	Widget
	ResourceContainer
	Init() error
	ShowPopup(widget MaxSizeWidget)
	FocusManager() FocusManager
	Update()
}

type NamespaceAccessor interface {
	CurrentNamespace() string
}

type ResourceContainer interface {
	NamespaceAccessor
	ErrorHandler
	Client() Client
	ResourceProvider() ResourceProvider
	CommandBuilder() CommandBuilder
	CommandExecutor() CommandExecutor
}
