package cmd

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pkg/errors"
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

	mux := &sync.Mutex{}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	// flag to ignore errors when killing process
	killing := false

	go func(m *sync.Mutex, cmd *exec.Cmd) {
		<-sigs
		m.Lock()
		killing = true
		pid := cmd.Process.Pid
		m.Unlock()
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if err != nil {
			panic(err)
		}
	}(mux, cmd)

	err := cmd.Run()
	signal.Reset(syscall.SIGINT)
	mux.Lock()
	defer mux.Unlock()

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
