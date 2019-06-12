package kube

import (
	"fmt"
	"strings"
)

func Describe(resType string, resName string) string {
	return kubectl(fmt.Sprintf("describe %s %s", resType, resName))
}

func DescribeNs(namespace string, resType string, resName string) string {
	return kubectlNs(namespace, fmt.Sprintf("describe %s %s", resType, resName))
}

func Edit(resType string, resName string) string {
	return kubectl(fmt.Sprintf("edit %s %s", resType, resName))
}

func EditNs(namespace string, resType string, resName string) string {
	return kubectlNs(namespace, fmt.Sprintf("edit %s %s", resType, resName))
}

func PortForward(namespace string, pod string, port string) string {
	return kubectlNs(namespace, fmt.Sprintf("port-forward %s %s", pod, port))
}

func Exec(namespace string, pod string, container string, command string) string {
	if container != "" {
		container = "-c " + container
	}
	return kubectlNs(namespace, fmt.Sprintf("exec -ti %s %s %s", container, pod, command))
}

func Logs(namespace string, pod string, container string, tail int, follow bool) string {
	var flags []string
	if container != "" {
		flags = append(flags, "-c "+container)
	}
	if tail > 0 {
		flags = append(flags, fmt.Sprintf("--tail=%d", tail))
	}
	if follow {
		flags = append(flags, "--follow")
	}
	return kubectlNs(namespace, fmt.Sprintf("logs %s %s", strings.Join(flags, " "), pod))
}

func Viewer(command string) string {
	return fmt.Sprintf("%s | less", command)
}

func kubectl(command string) string {
	context := ""
	if config.Context != "" {
		context = " --context " + config.Context
	}
	return fmt.Sprintf("KUBECONFIG=%s kubectl%s %s", config.Path, context, command)
}

func kubectlNs(namespace string, command string) string {
	return fmt.Sprintf(kubectl("--namespace %s %s"), namespace, command)
}
