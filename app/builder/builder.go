package builder

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	"strconv"
	"strings"
)

type builder struct {
	config      commander.Config
	kubectlBin  string
	pagerBin    string
	logPagerBin string
	editorBin   string
	tail        int
}

func NewBuilder(
	config commander.Config,
	kubectl string,
	pager string,
	logPager string,
	editor string,
	tail int,
) *builder {
	return &builder{
		config:      config,
		kubectlBin:  kubectl,
		pagerBin:    pager,
		logPagerBin: logPager,
		editorBin:   editor,
		tail:        tail,
	}
}

func (b builder) Describe(namespace string, resType string, resName string) *commander.Command {
	return b.kubectl(namespace, "describe", resType, resName)
}

func (b builder) Edit(namespace string, resType string, resName string) *commander.Command {
	cmd := b.kubectl(namespace, "edit", resType, resName)
	if b.editorBin != "" {
		cmd = cmd.WithEnv("EDITOR", b.editorBin)
	}
	return cmd
}

func (b builder) PortForward(namespace string, pod string, port int32) *commander.Command {
	return b.kubectl(namespace, "port-forward", pod, strconv.Itoa(int(port)))
}

func (b builder) Exec(namespace string, pod string, container string, command string) *commander.Command {
	args := []string{"exec", "-ti"}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, pod, "--", command)
	return b.kubectl(namespace, args...)
}

func (b builder) Logs(namespace string, pod string, container string, previous bool, follow bool) *commander.Command {
	args := []string{"logs"}
	if container != "" {
		args = append(args, "-c", container)
	}
	if b.tail > 0 {
		args = append(args, "--tail", strconv.Itoa(b.tail))
	}
	if follow {
		args = append(args, "--follow")
	}
	if previous {
		args = append(args, "--previous")
	}
	args = append(args, pod)
	return b.kubectl(namespace, args...)
}

func (b builder) Pager() []*commander.Command {
	return b.binCommand(b.pagerBin)
}

func (b builder) LogPager() []*commander.Command {
	return b.binCommand(b.logPagerBin)
}

func (b builder) binCommand(bin string) []*commander.Command {
	if bin == "" {
		return nil
	}
	commandsStr := strings.Split(bin, "|")
	var commands []*commander.Command
	for _, str := range commandsStr {
		parts := strings.Fields(str)
		commands = append(commands, commander.NewCommand(parts[0], parts[1:]...))
	}
	return commands
}

func (b builder) kubectl(namespace string, command ...string) *commander.Command {
	var args []string
	if context := b.config.Context(); context != "" {
		args = append(args, "--context", context)
	}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	args = append(args, command...)
	c := commander.NewCommand(b.kubectlBin, args...)
	if kubeconfig := b.config.Kubeconfig(); kubeconfig != "" {
		c.WithEnv("KUBECONFIG", kubeconfig)
	}
	return c
}
