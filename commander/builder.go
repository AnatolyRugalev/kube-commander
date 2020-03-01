package commander

type CommandBuilder interface {
	Describe(namespace string, resType string, resName string) *Command
	Edit(namespace string, resType string, resName string) *Command
	PortForward(namespace string, pod string, port string) *Command
	Exec(namespace string, pod string, container string, command string) *Command
	Logs(namespace string, pod string, container string, tail int, follow bool) *Command
	Viewer() *Command
}
