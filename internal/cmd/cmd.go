package cmd

import (
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func Execute(name string, arg ...string) error {
	// let's group processes into one process group
	// to avoid zombie child processes
	// https://medium.com/@felixge/killing-a-child-process-and-all-of-its-children-in-go-54079af94773
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Noctty:  true,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	// flag to ignore errors when killing process
	killing := false
	go func() {
		<-sigs
		killing = true
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err != nil {
			panic(err)
		}
	}()
	err := cmd.Run()
	signal.Reset(syscall.SIGINT)
	if killing {
		return nil
	}
	return err
}

func Shell(command string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return errors.New("SHELL env is not set")
	}
	return Execute(shell, "-c", command)
}
