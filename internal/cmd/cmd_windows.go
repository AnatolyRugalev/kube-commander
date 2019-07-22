// +build windows

package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
)

func createCmd(name string, arg []string) *exec.Cmd {
	return exec.Command(name, arg...)
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
