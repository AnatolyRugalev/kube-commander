package executor

import (
	"bytes"
	"errors"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
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
	output := bytes.Buffer{}
	cmd := e.createCmd(command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	// flag to ignore errors when killing process
	var (
		killing    bool
		commandPid int
	)

	go func(cmd *exec.Cmd) {
		sig := <-sigs
		if sig == nil {
			return
		}
		e.Lock()
		defer e.Unlock()
		killing = true
		_ = e.killProcessGroup(commandPid)
	}(cmd)

	started := time.Now()
	err := cmd.Start()
	if err != nil {
		return &commander.ExecErr{
			Err:    err,
			Output: output.Bytes(),
		}
	}
	e.Lock()
	commandPid = cmd.Process.Pid
	e.Unlock()

	err = cmd.Wait()
	signal.Reset(syscall.SIGINT)
	close(sigs)

	e.Lock()
	defer e.Unlock()
	if killing {
		return nil
	}
	if err != nil {
		return &commander.ExecErr{
			Err:    err,
			Output: output.Bytes(),
		}
	}
	if time.Now().Sub(started) < time.Second {
		return &commander.ExecErr{
			Err:    errors.New("command exited too early"),
			Output: output.Bytes(),
		}
	}
	return nil
}
