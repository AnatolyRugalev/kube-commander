// +build windows

package executor

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"os/exec"
	"strconv"
)

func NewOsExecutor() *executor {
	return NewExecutor("PowerShell")
}

func (e executor) createCmd(command *commander.Command) *exec.Cmd {
	return command.ToCmd()
}

func (e executor) renderCommand(command *commander.Command) string {
	var env []string
	for name, value := range command.Envs() {
		env = append(env, fmt.Sprintf("$env:%s = '%s'", name, value))
	}
	return strings.Join(env, "; ") + " " + command.Name() + strings.Join(command.Args(), " ")
}

func (e executor) killProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return kill.Run()
}
