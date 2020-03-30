package executor

import (
	"bufio"
	"bytes"
	"fmt"
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

func (e *executor) Pipe(command ...*commander.Command) error {
	var strCmd []string
	for _, c := range command {
		strCmd = append(strCmd, e.renderCommand(c))
	}
	return e.execute(commander.NewCommand(e.shell, "-c", strings.Join(strCmd, " | ")))
}

func (e *executor) execute(command *commander.Command) error {
	e.Lock()
	defer e.Unlock()

	_, _ = fmt.Fprintf(os.Stdout, "\n=========================\n")
	_, _ = fmt.Fprintf(os.Stdout, "Executing command: %s\n", e.renderCommand(command))
	stderr := bytes.Buffer{}
	cmd := e.createCmd(command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	err := cmd.Start()
	if err != nil {
		return &commander.ExecErr{
			Err:    fmt.Errorf("could not start process: %w", err),
			Output: stderr.Bytes(),
		}
	}
	commandPid := cmd.Process.Pid
	killed := false
	go func(cmd *exec.Cmd) {
		_, ok := <-sigs
		if !ok {
			return
		}
		killed = true
		_ = e.interruptProcess(commandPid)
	}(cmd)

	err = cmd.Wait()
	signal.Stop(sigs)
	close(sigs)
	if killed {
		return nil
	} else if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error executing command: %s\n", err.Error())
		_, _ = fmt.Fprintf(os.Stderr, "Press Enter to continue...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_, _ = fmt.Fprintf(os.Stdout, "=========================\n")

		return &commander.ExecErr{
			Err:    fmt.Errorf("error executing command: %w", err),
			Output: stderr.Bytes(),
		}
	}
	return nil
}
