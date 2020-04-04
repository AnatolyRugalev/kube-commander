package pod

import (
	"errors"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
)

type ContainerFunc func(pod v1.Pod, container v1.Container, status v1.ContainerStatus)

func pickPodContainer(workspace commander.Workspace, pod v1.Pod, f ContainerFunc) {
	if len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses) == 0 {
		workspace.Status().Error(errors.New("no containers available"))
		return
	}
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

func (i item) Id() string {
	return i.container.Name
}

func (i item) Cells() []string {
	return []string{i.container.Name, containerState(i.status.State)}
}

func (i item) Enabled() bool {
	return true
}

type picker struct {
	*listTable.ListTable
	pod v1.Pod
	f   ContainerFunc
}

func newContainerPicker(pod v1.Pod, f ContainerFunc) *picker {
	var items []commander.Row
	for i, status := range pod.Status.InitContainerStatuses {
		items = append(items, &item{
			container: pod.Spec.Containers[i],
			status:    status,
		})
	}
	for i, status := range pod.Status.ContainerStatuses {
		items = append(items, &item{
			container: pod.Spec.Containers[i],
			status:    status,
		})
	}
	picker := &picker{
		ListTable: listTable.NewStaticListTable([]string{"Container", "Status"}, items, listTable.WithHeaders),
		pod:       pod,
		f:         f,
	}
	picker.BindOnKeyPress(picker.OnKeyPress)
	return picker
}

func (p *picker) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		item, ok := row.(*item)
		if ok {
			go p.f(p.pod, item.container, item.status)
		}
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
