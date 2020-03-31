package pod

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
	"strconv"
)

type PortFunc func(pod v1.Pod, container v1.Container, port v1.ContainerPort)

func pickPodPort(workspace commander.Workspace, pod v1.Pod, f PortFunc) {
	if len(pod.Spec.Containers) == 1 && len(pod.Spec.Containers[0].Ports) == 1 {
		f(pod, pod.Spec.Containers[0], pod.Spec.Containers[0].Ports[0])
		return
	}
	picker := newPortPicker(pod, func(pod v1.Pod, c v1.Container, port v1.ContainerPort) {
		workspace.FocusManager().Blur()
		f(pod, c, port)
	})
	workspace.ShowPopup("Select container", picker)
}

type portItem struct {
	container v1.Container
	port      v1.ContainerPort
}

type portPicker struct {
	*listTable.ListTable
	pod   v1.Pod
	items map[string]*portItem
	f     PortFunc
}

func newPortPicker(pod v1.Pod, f PortFunc) *portPicker {
	var items []*portItem
	for i, container := range pod.Spec.Containers {
		// Skip non-running containers
		if pod.Status.ContainerStatuses[i].State.Running == nil {
			continue
		}
		for _, port := range container.Ports {
			items = append(items, &portItem{
				container: container,
				port:      port,
			})
		}
	}
	var rows []commander.Row
	itemMap := make(map[string]*portItem)
	for _, c := range items {
		var portStr string
		if c.port.Name != "" {
			portStr = fmt.Sprintf("%s (%d)", c.port.Name, c.port.ContainerPort)
		} else {
			portStr = strconv.Itoa(int(c.port.ContainerPort))
		}

		itemId := c.container.Name + ":" + portStr
		rows = append(rows, commander.NewSimpleRow(itemId, []string{c.container.Name, portStr}))
		itemMap[itemId] = c
	}
	picker := &portPicker{
		ListTable: listTable.NewStaticListTable([]string{"Container", "Port"}, rows, listTable.WithHeaders),
		pod:       pod,
		items:     itemMap,
		f:         f,
	}
	picker.BindOnKeyPress(picker.OnKeyPress)
	return picker
}

func (p *portPicker) OnKeyPress(row commander.Row, event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		item := p.items[row.Id()]
		go p.f(p.pod, item.container, item.port)
		return true
	}
	return false
}
