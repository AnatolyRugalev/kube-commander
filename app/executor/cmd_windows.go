// +build windows

package executor

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func NewOsExecutor() *executor {
	return NewExecutor("PowerShell")
}

func (e *executor) renderCommand(command *commander.Command) string {
	var env []string
	for name, value := range command.Envs() {
		env = append(env, fmt.Sprintf("$env:%s='%s';", name, value))
	}
	return strings.Join(env, "") + " " + command.Name() + " " + strings.Join(command.Args(), " ")
}

func (e *executor) runCmd(cmd *exec.Cmd) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return &commander.ExecErr{
			Err: fmt.Errorf("could not start process: %w", err),
		}
	}
	commandPid := cmd.Process.Pid
	killed := false
	go func(cmd *exec.Cmd) {
		_, ok := <-sigCh
		if !ok {
			return
		}
		killed = true
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(commandPid)).Run()
	}(cmd)
	err = cmd.Wait()
	if err != nil && !killed {
		return err
	}
	return nil
}
