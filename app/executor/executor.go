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

	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"
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
	stderr := bytes.Buffer{}

	_, _ = fmt.Fprintf(os.Stdout, "\n=========================\n")
	_, _ = fmt.Fprintf(os.Stdout, "Executing command: %s\n", e.renderCommand(command))

	cmd := command.ToCmd()

	// Start the command with a pty.
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return &commander.ExecErr{
			Err:    fmt.Errorf("could not start terminal: %w", err),
			Output: stderr.Bytes(),
		}
	}
	// Make sure to close the pty at the end.
	defer func() { _ = ptmx.Close() }() // Best effort.

	// Handle pty size.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.

	// Set stdin in raw mode.
	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = terminal.Restore(int(os.Stdin.Fd()), oldState) }() // Best effort.

	// Copy stdin to the pty and the pty to stdout.
	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, ptmx)

	return nil
}
