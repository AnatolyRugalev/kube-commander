package commander

type App interface {
	Container
	Run() error
	Update()
	Quit()
}

type Container interface {
	Client() Client
	ResourceProvider() ResourceProvider
	CommandBuilder() CommandBuilder
	CommandExecutor() CommandExecutor
	Screen() Screen
	ErrorHandler() ErrorHandler
}
