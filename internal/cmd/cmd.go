package cmd

import (
	"github.com/pkg/errors"
	"os"
	"os/exec"
)

func Execute(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Shell(command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return errors.New("SHELL env is not set")
	}
	return Execute(shell, "-c", command)
}
