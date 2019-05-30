package kube

import (
	"fmt"
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
