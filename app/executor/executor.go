package executor

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type executor struct {
	shell string
	sync.Mutex
}

func NewExecutor(shell string) *executor {
	return &executor{
		shell: shell,
	}
}

func (e executor) Pipe(command ...*commander.Command) error {
	var strCmd []string
	for _, c := range command {
		strCmd = append(strCmd, e.renderCommand(c))
	}
	return e.Execute(commander.NewCommand(e.shell, "-c", strings.Join(strCmd, " | ")))
}

// TODO: fix raw Execute calls. Only Pipe works for some reason (exit code: 1)
func (e executor) Execute(command *commander.Command) error {
	cmd := e.createCmd(command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	// flag to ignore errors when killing process
	var (
		killing    bool
		commandPid int
	)

	go func(cmd *exec.Cmd) {
		<-sigs
		e.Lock()
		defer e.Unlock()
		killing = true
		_ = e.killProcessGroup(commandPid)
	}(cmd)

	err := cmd.Start()
	if err != nil {
		return err
	}
	e.Lock()
	commandPid = cmd.Process.Pid
	e.Unlock()

	err = cmd.Wait()
	signal.Reset(syscall.SIGINT)

	e.Lock()
	defer e.Unlock()
	if killing {
		return nil
	}
	return err
}
