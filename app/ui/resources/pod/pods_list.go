package pod

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"k8s.io/api/core/v1"
)

type PodsList struct {
	*listTable.ResourceListTable

	workspace commander.Workspace
	resource  *commander.Resource
}

func NewPodsList(workspace commander.Workspace, resource *commander.Resource, opts *listTable.ResourceListTableOptions) *PodsList {
	pl := PodsList{
		ResourceListTable: listTable.NewResourceListTable(workspace, resource, opts),
		workspace:         workspace,
		resource:          resource,
	}
	pl.BindOnKeyPress(pl.OnKeyPress)
	return &pl
}

func (p PodsList) OnKeyPress(rowId int, _ commander.Row, event *tcell.EventKey) bool {
	switch event.Rune() {
	case 'L', 'l':
		go p.logs(rowId)
		return true
	}
	return false
}

func (p PodsList) logs(rowId int) {
	metadata, err := p.RowMetadata(rowId)
	if err != nil {
		p.workspace.HandleError(err)
		return
	}
	pod := v1.Pod{}
	err = p.workspace.Client().Get(p.resource, metadata.Namespace, metadata.Name, &pod)
	if err != nil {
		p.workspace.HandleError(err)
		return
	}
	pickPodContainer(p.workspace, pod, func(pod v1.Pod, container v1.Container, status v1.ContainerStatus) {
		e := p.workspace.CommandExecutor()
		b := p.workspace.CommandBuilder()
		var commands []*commander.Command
		if status.State.Running != nil {
			commands = append(commands, b.Logs(pod.Namespace, pod.Name, container.Name, 1000, true))
		} else {
			commands = append(commands, b.Logs(pod.Namespace, pod.Name, container.Name, 1000, false), b.Viewer())
		}
		err := e.Pipe(commands...)
		if err != nil {
			p.workspace.HandleError(err)
			return
		}
		return
	})
}
