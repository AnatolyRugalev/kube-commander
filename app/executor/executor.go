package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/AnatolyRugalev/kube-commander/commander"
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

// execute runs given command in emulated PTY environment
// This is required for cross-platform execution
func (e *executor) execute(command *commander.Command) error {
	stderr := bytes.Buffer{}

	_, _ = fmt.Fprintf(os.Stdout, "\n=========================\n")
	_, _ = fmt.Fprintf(os.Stdout, "Executing command: %s\n", e.renderCommand(command))

	cmd := command.ToCmd()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return &commander.ExecErr{
			Err:    fmt.Errorf("could not start terminal: %w", err),
			Output: stderr.Bytes(),
		}
	}
	defer func() { _ = ptmx.Close() }()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	// syscall.SetNonBlock is a workaround, read below
	err = syscall.SetNonblock(int(os.Stdin.Fd()), false)
	if err != nil {
		return err
	}
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }()

	// Copy stdin to the pty and the pty to stdout.
	go func() {
		_, err = io.Copy(ptmx, os.Stdin)
	}()
	_, _ = io.Copy(os.Stdout, ptmx)
	// syscall.SetNonBlock is a workaround to stop copying from os.Stdin immediately when child ptmx is closing.
	// This workaround is needed to prevent next os.Stdin byte to be eaten by copying to ptmx
	err = syscall.SetNonblock(int(os.Stdin.Fd()), true)
	if err != nil {
		return err
	}

	return nil
}
