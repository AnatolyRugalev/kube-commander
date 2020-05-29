// +build !windows

package executor

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func NewOsExecutor() *executor {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return NewExecutor(shell)
}

func (e *executor) createCmd(command *commander.Command) *exec.Cmd {
	// let's group processes into one process group
	// to avoid zombie child processes
	// https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773
	cmd := command.ToCmd()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Noctty:  true,
	}
	return cmd
}

func (e *executor) renderCommand(command *commander.Command) string {
	var env []string
	for name, value := range command.Envs() {
		env = append(env, name+"="+value)
	}
	return strings.Join(append(env, command.Name()), " ") + " " + strings.Join(command.Args(), " ")
}

func (e *executor) interruptProcess(pid int) error {
	// -pid to kill a whole group
	return syscall.Kill(-pid, syscall.SIGINT)
}
