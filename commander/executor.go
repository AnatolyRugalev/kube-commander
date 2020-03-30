package commander

import "os/exec"

type ExecErr struct {
	Err    error
	Output []byte
}

func (e ExecErr) Error() string {
	return e.Err.Error()
}

type CommandExecutor interface {
	Pipe(command ...*Command) error
}

type Command struct {
	name string
	args []string
	envs map[string]string
}

func (c Command) Envs() map[string]string {
	return c.envs
}

func (c Command) Args() []string {
	return c.args
}

func (c Command) Name() string {
	return c.name
}

func NewCommand(name string, arg ...string) *Command {
	return &Command{
		name: name,
		args: arg,
		envs: make(map[string]string),
	}
}

func (c *Command) WithEnv(name, value string) *Command {
	c.envs[name] = value
	return c
}

func (c Command) ToCmd() *exec.Cmd {
	cmd := exec.Command(c.Name(), c.Args()...)
	for name, val := range c.Envs() {
		cmd.Env = append(cmd.Env, name+"="+val)
	}
	return cmd
}
