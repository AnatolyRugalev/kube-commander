package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodsTable struct {
	namespace string
}

func (pt *PodsTable) TypeName() string {
	return "pod"
}

func (pt *PodsTable) Name(item []string) string {
	return item[0]
}

func (pt *PodsTable) Namespace() string {
	return pt.namespace
}

func (pt *PodsTable) OnEvent(event *termui.Event, item []string) bool {
	switch event.ID {
	case "l":
		var cmd string
		if item[2] == "Running" {
			cmd = kube.Logs(pt.namespace, item[0], "", 1000, true)
		} else {
			cmd = kube.Viewer(kube.Logs(pt.namespace, item[0], "", 1000, false))
		}
		screen.SwitchToCommand(cmd)
		return true
	case "x":
		screen.SwitchToCommand(kube.Exec(pt.namespace, item[0], "", "/bin/sh -- -c '/bin/bash || /bin/sh'"))
		return true
	}
	return false
}

func NewPodsTable(namespace string) *widgets.ListTable {
	lt := widgets.NewListTable(screen, &PodsTable{
		namespace: namespace,
	})
	lt.Title = "Pods <" + namespace + ">"
	return lt
}

func (pt *PodsTable) GetHeaderRow() []string {
	return []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
}

func (pt *PodsTable) LoadData() ([][]string, error) {
	client, err := kube.GetClient()
	if err != nil {
		return nil, err
	}
	pods, err := client.CoreV1().Pods(pt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, pod := range pods.Items {
		rows = append(rows, pt.newRow(pod))
	}
	return rows, nil
}

func (pt *PodsTable) newRow(pod v1.Pod) []string {
	var total, ready, restarts int32

	for _, c := range pod.Status.ContainerStatuses {
		total++
		restarts += c.RestartCount
		if c.Ready {
			ready++
		}
	}
	return []string{
		pod.Name,
		fmt.Sprintf("%d/%d", ready, total),
		string(pod.Status.Phase),
		fmt.Sprintf("%d", restarts),
		Age(pod.CreationTimestamp.Time),
	}
}

func (pt *PodsTable) OnDelete(item []string) bool {
	name := item[0]
	ShowConfirmDialog("Are you sure you want to delete pod "+name+"?", func() error {
		client, err := kube.GetClient()
		if err != nil {
			return err
		}

		err = client.CoreV1().Pods(pt.namespace).Delete(name, metav1.NewDeleteOptions(0))
		if err != nil {
			return err
		}
		return nil
	})
	return true
}
