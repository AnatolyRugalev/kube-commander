package pod

import (
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	v1 "k8s.io/api/core/v1"
)

type ContainerFunc func(pod *v1.Pod, container *v1.Container)

func pickPodContainer(workspace commander.Workspace, pod *v1.Pod, f ContainerFunc) {
	if len(pod.Spec.Containers) == 1 {
		f(pod, &pod.Spec.Containers[0])
		return
	}
	picker, err := newContainerPicker(pod, func(pod *v1.Pod, c *v1.Container) {
		workspace.FocusManager().Blur()
		f(pod, c)
	})
	if err != nil {
		workspace.HandleError(err)
		return
	}
	workspace.ShowPopup(picker)
}

func newContainerPicker(pod *v1.Pod, f ContainerFunc) (*listTable.ListTable, error) {
	var items []string
	for _, container := range pod.Spec.Containers {
		items = append(items, container.Name)
	}

	lt := listTable.NewList(items)
	lt.BindOnKeyPress(func(rowId int, row commander.Row, event *tcell.EventKey) bool {
		if event.Key() == tcell.KeyEnter {
			go f(pod, &pod.Spec.Containers[rowId])
			return true
		}
		return false
	})
	return lt, nil
}
