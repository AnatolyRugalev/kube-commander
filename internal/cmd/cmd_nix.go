// +build !windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

func createCmd(name string, arg []string) *exec.Cmd {
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
	return cmd
}

func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
