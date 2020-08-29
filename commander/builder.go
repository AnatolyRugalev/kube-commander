package commander

type CommandBuilder interface {
	Describe(namespace string, resType string, resName string) *Command
	Edit(namespace string, resType string, resName string) *Command
	PortForward(namespace string, pod string, port int32) *Command
	Exec(namespace string, pod string, container string, command string) *Command
	Logs(namespace string, pod string, container string, previous bool, follow bool) *Command
	Pager() *Command
}
