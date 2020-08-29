// +build !windows

package executor

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/creack/pty"
	"golang.org/x/crypto/ssh/terminal"
	"io"
	"os"
	"os/exec"
	"os/signal"
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

func (e *executor) renderCommand(command *commander.Command) string {
	var env []string
	for name, value := range command.Envs() {
		env = append(env, name+"="+value)
	}
	return strings.Join(append(env, command.Name()), " ") + " " + strings.Join(command.Args(), " ")
}

// execute runs given command in emulated PTY environment
// This is required for cross-platform execution
func (e *executor) runCmd(cmd *exec.Cmd) error {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("could not start PTY terminal: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	defer signal.Stop(resize)
	// syscall.SetNonBlock is a workaround, read below
	err = syscall.SetNonblock(int(os.Stdin.Fd()), false)
	if err != nil {
		return err
	}
	go func() {
		for range resize {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				_, _ = fmt.Fprintf(os.Stdout, "error resizing pty: %s", err)
			}
		}
	}()
	resize <- syscall.SIGWINCH
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
	_, err = cmd.Process.Wait()
	return err
}
