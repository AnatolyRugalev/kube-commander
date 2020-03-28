package pod

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
)

type ContainerFunc func(pod v1.Pod, container v1.Container, status v1.ContainerStatus)

func pickPodContainer(workspace commander.Workspace, pod v1.Pod, f ContainerFunc) {
	if len(pod.Spec.Containers) == 1 {
		f(pod, pod.Spec.Containers[0], pod.Status.ContainerStatuses[0])
		return
	}
	picker := newContainerPicker(pod, func(pod v1.Pod, c v1.Container, status v1.ContainerStatus) {
		workspace.FocusManager().Blur()
		f(pod, c, status)
	})
	workspace.ShowPopup("Select container", picker)
}

type item struct {
	container v1.Container
	status    v1.ContainerStatus
}

type picker struct {
	*listTable.ListTable
	pod   v1.Pod
	items map[string]*item
	f     ContainerFunc
}

func newContainerPicker(pod v1.Pod, f ContainerFunc) *picker {
	var items []*item
	for i, container := range pod.Spec.InitContainers {
		items = append(items, &item{
			container: container,
			status:    pod.Status.InitContainerStatuses[i],
		})
	}
	for i, container := range pod.Spec.Containers {
		items = append(items, &item{
			container: container,
			status:    pod.Status.ContainerStatuses[i],
		})
	}
	var rows []commander.Row
	itemMap := make(map[string]*item)
	for _, c := range items {
		rows = append(rows, commander.NewSimpleRow(c.container.Name, []string{c.container.Name, containerState(c.status.State)}))
		itemMap[c.container.Name] = c
	}
	picker := &picker{
		ListTable: listTable.NewStaticListTable([]string{"Container", "Status"}, rows, listTable.WithHeaders),
		pod:       pod,
		items:     itemMap,
		f:         f,
	}
	picker.BindOnKeyPress(picker.OnKeyPress)
	return picker
}

func (p *picker) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		item := p.items[row.Id()]
		go p.f(p.pod, item.container, item.status)
		return true
	}
	return false
}

func containerState(state v1.ContainerState) string {
	if state.Running != nil {
		return "Running"
	} else if state.Waiting != nil {
		return "Waiting"
	} else if state.Terminated != nil {
		return "Terminated"
	} else {
		return "Unknown"
	}
}
