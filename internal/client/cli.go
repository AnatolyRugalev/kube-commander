package client

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/cmd"
	"strings"
)

func (c client) Describe(namespace string, resType string, resName string) string {
	return c.kubectl(namespace, fmt.Sprintf("describe %s %s", resType, resName))
}

func (c client) Edit(namespace string, resType string, resName string) string {
	return c.kubectl(namespace, fmt.Sprintf("edit %s %s", resType, resName))
}

func (c client) PortForward(namespace string, pod string, port string) string {
	return c.kubectl(namespace, fmt.Sprintf("port-forward %s %s", pod, port))
}

func (c client) Exec(namespace string, pod string, container string, command string) string {
	if container != "" {
		container = "-c " + container
	}
	return c.kubectl(namespace, fmt.Sprintf("exec -ti %s %s %s", container, pod, command))
}

func (c client) Logs(namespace string, pod string, container string, tail int, follow bool) string {
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
	return c.kubectl(namespace, fmt.Sprintf("logs %s %s", strings.Join(flags, " "), pod))
}

func (c client) Viewer(command string) string {
	return fmt.Sprintf("%s | less", command)
}

func (c client) kubectl(namespace, command string) string {
	if context := c.provider.Context(); context != "" {
		context = " --context " + context
	}
	if namespace != "" {
		namespace = " --namespace %s %s"
	}
	command = fmt.Sprintf("kubectl%s %s", namespace, command)
	if kubeconfig := c.provider.Kubeconfig(); kubeconfig != "" {
		command = cmd.AppendEnv("KUBECONFIG", kubeconfig, command)
	}
	return command
}
