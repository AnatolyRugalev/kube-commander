// +build windows

package cmd

import (
	"os"
	"os/exec"
	"strconv"
)

func createCmd(name string, arg []string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func killProcessGroup(pid int) error {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return kill.Run()
}
