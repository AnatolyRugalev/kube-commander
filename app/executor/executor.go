package executor

import (
	"bufio"
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"os"
	"strings"
	"sync"
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
	pipeCmd := commander.NewCommand(e.shell, "-c", strings.Join(strCmd, " | "))
	return e.executeCommand(pipeCmd)
}

func (e *executor) executeCommand(command *commander.Command) error {
	e.Lock()
	defer e.Unlock()
	_, _ = fmt.Fprintf(os.Stdout, "\n=========================\n")
	_, _ = fmt.Fprintf(os.Stdout, "Executing command: %s\n", e.renderCommand(command))

	err := e.runCmd(command.ToCmd())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error executing command: %s\n", err.Error())
		_, _ = fmt.Fprintf(os.Stderr, "Press Enter to continue...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_, _ = fmt.Fprintf(os.Stdout, "=========================\n")

		return &commander.ExecErr{
			Err: fmt.Errorf("error executing command: %w", err),
		}
	}
	return nil
}
