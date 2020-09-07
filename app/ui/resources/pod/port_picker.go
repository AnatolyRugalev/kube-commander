package pod

import (
	"errors"
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
	"strconv"
)

type PortFunc func(pod v1.Pod, container v1.Container, port v1.ContainerPort)

func pickPodPort(workspace commander.Workspace, pod v1.Pod, f PortFunc) {
	picker, err := newPortPicker(workspace.ScreenHandler(), pod, func(pod v1.Pod, c v1.Container, port v1.ContainerPort) {
		workspace.FocusManager().Blur()
		f(pod, c, port)
	})
	if err != nil {
		workspace.Status().Error(err)
		return
	}
	workspace.ShowPopup("Select container port", picker)
}

type portItem struct {
	container v1.Container
	status    v1.ContainerStatus
	port      v1.ContainerPort
}

func (p portItem) Id() string {
	return fmt.Sprintf("%s:%d", p.container.Name, p.port.ContainerPort)
}

func (p portItem) Cells() []string {
	return []string{p.container.Name, containerState(p.status.State), strconv.Itoa(int(p.port.ContainerPort))}
}

func (p portItem) Enabled() bool {
	return p.status.State.Running != nil
}

type portPicker struct {
	*listTable.ListTable
	pod v1.Pod
	f   PortFunc
}

func newPortPicker(screen commander.ScreenHandler, pod v1.Pod, f PortFunc) (*portPicker, error) {
	var items []commander.Row
	for i, status := range pod.Status.ContainerStatuses {
		container := pod.Spec.Containers[i]
		for _, port := range container.Ports {
			items = append(items, &portItem{
				container: container,
				status:    status,
				port:      port,
			})
		}
	}
	if len(items) == 0 {
		return nil, errors.New("this pod doesn't have any defined or active ports")
	}
	picker := &portPicker{
		ListTable: listTable.NewStaticListTable([]string{"Container", "Status", "Port"}, items, listTable.WithHeaders, screen),
		pod:       pod,
		f:         f,
	}
	picker.BindOnKeyPress(picker.OnKeyPress)
	return picker, nil
}

func (p *portPicker) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		item, ok := row.(*portItem)
		if ok {
			go p.f(p.pod, item.container, item.port)
		}
		return true
	}
	return false
}
