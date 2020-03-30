package pod

import (
	"errors"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"k8s.io/api/core/v1"
	"strings"
)

type PodsList struct {
	*listTable.ResourceListTable

	workspace commander.Workspace
	resource  *commander.Resource
}

func NewPodsList(workspace commander.Workspace, resource *commander.Resource, format listTable.TableFormat, updater commander.ScreenUpdater) *PodsList {
	pl := PodsList{
		ResourceListTable: listTable.NewResourceListTable(workspace, resource, format, updater),
		workspace:         workspace,
		resource:          resource,
	}
	pl.BindOnKeyPress(pl.OnKeyPress)
	return &pl
}

func (p PodsList) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	switch event.Rune() {
	case 'l':
		go p.logs(row)
		return true
	case 's':
		go p.shell(row)
		return true
	}
	return false
}

func (p PodsList) getPod(row commander.Row) (*v1.Pod, error) {
	metadata, err := p.RowMetadata(row)
	if err != nil {
		return nil, err
	}
	pod := v1.Pod{}
	err = p.workspace.Client().Get(p.resource, metadata.Namespace, metadata.Name, &pod)
	if err != nil {
		return nil, err
	}
	return &pod, nil
}

func (p PodsList) logs(row commander.Row) {
	pod, err := p.getPod(row)
	if err != nil {
		p.workspace.HandleError(err)
		return
	}
	pickPodContainer(p.workspace, *pod, func(pod v1.Pod, container v1.Container, status v1.ContainerStatus) {
		e := p.workspace.CommandExecutor()
		b := p.workspace.CommandBuilder()
		var commands []*commander.Command
		if status.State.Running != nil {
			commands = append(commands, b.Logs(pod.Namespace, pod.Name, container.Name, 1000, true))
		} else {
			commands = append(commands, b.Logs(pod.Namespace, pod.Name, container.Name, 1000, false), b.Pager())
		}
		err := e.Pipe(commands...)
		if err != nil {
			p.workspace.HandleError(err)
			return
		}
		return
	})
}

func (p PodsList) shell(row commander.Row) {
	pod, err := p.getPod(row)
	if err != nil {
		p.workspace.HandleError(err)
		return
	}
	pickPodContainer(p.workspace, *pod, func(pod v1.Pod, container v1.Container, status v1.ContainerStatus) {
		e := p.workspace.CommandExecutor()
		b := p.workspace.CommandBuilder()
		err = e.Pipe(b.Exec(pod.Namespace, pod.Name, container.Name, "/bin/sh"))
		if err != nil {
			if execErr, ok := err.(*commander.ExecErr); ok {
				if strings.Contains(string(execErr.Output), "no such file or directory") {
					err = errors.New("this container doesn't have /bin/sh")
				}
			}
		}
		if err != nil {
			p.workspace.HandleError(err)
			return
		}
		return
	})
}
