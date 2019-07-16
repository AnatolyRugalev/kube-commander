// +build windows

package cmd

import (
	"fmt"
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

func Shell(command string) error {
	return Execute("PowerShell", "-c", command)
}

func AppendEnv(name, value, command string) string {
	return fmt.Sprintf("$env:%s = '%s'; %s", name, value, command)
}
