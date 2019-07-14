package cmd

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
)

func Execute(name string, arg ...string) error {
	cmd := createCmd(name, arg)

	mux := &sync.Mutex{}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	// flag to ignore errors when killing process
	var (
		killing    bool
		commandPid int
	)

	go func(m *sync.Mutex, cmd *exec.Cmd) {
		<-sigs
		m.Lock()
		defer m.Unlock()
		killing = true
		err := killProcessGroup(commandPid)
		if err != nil {
			panic(err)
		}
	}(mux, cmd)

	err := cmd.Start()
	if err != nil {
		return err
	}
	mux.Lock()
	commandPid = cmd.Process.Pid
	mux.Unlock()

	err = cmd.Wait()
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
