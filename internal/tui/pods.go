package tui

import (
	"fmt"

	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodsTable struct {
	namespace string
}

func (pt *PodsTable) Delete(row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Pods(pt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (pt *PodsTable) DeleteDescription(row widgets.ListRow) string {
	return "pod " + pt.namespace + "/" + row[0]
}

func (pt *PodsTable) TypeName() string {
	return "pod"
}

func (pt *PodsTable) Name(row widgets.ListRow) string {
	return row[0]
}

func (pt *PodsTable) Namespace() string {
	return pt.namespace
}

func (pt *PodsTable) GetActions() []*widgets.ListAction {
	return append(GetDefaultActions(pt),
		&widgets.ListAction{
			Name:          "Exec",
			HotKey:        "x",
			HotKeyDisplay: "X",
			Func:          pt.OnExec,
		},
		&widgets.ListAction{
			Name:          "Logs",
			HotKey:        "l",
			HotKeyDisplay: "L",
			Func:          pt.OnLogs,
		},
	)
}

func NewPodsTable(namespace string) *widgets.DataTable {
	pt := &PodsTable{
		namespace: namespace,
	}
	lt := widgets.NewDataTable(pt, screen)
	lt.Title = "Pods <" + namespace + ">"
	return lt
}

func (pt *PodsTable) GetHeaderRow() widgets.ListRow {
	return widgets.ListRow{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
}

func (pt *PodsTable) LoadData() ([]widgets.ListRow, error) {
	pods, err := kube.GetClient().CoreV1().Pods(pt.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows []widgets.ListRow
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
	return widgets.ListRow{
		pod.Name,
		fmt.Sprintf("%d/%d", ready, total),
		string(pod.Status.Phase),
		fmt.Sprintf("%d", restarts),
		Age(pod.CreationTimestamp.Time),
	}
}

func (pt *PodsTable) OnExec(handler widgets.ListTableHandler, row widgets.ListRow) bool {
	screen.SwitchToCommand(kube.Exec(pt.namespace, row[0], "", "/bin/sh -- -c '/bin/bash || /bin/sh'"))
	return true
}

func (pt *PodsTable) OnLogs(handler widgets.ListTableHandler, row widgets.ListRow) bool {
	var cmd string
	if row[2] == "Running" {
		cmd = kube.Logs(pt.namespace, row[0], "", 1000, true)
	} else {
		cmd = kube.Viewer(kube.Logs(pt.namespace, row[0], "", 1000, false))
	}
	screen.SwitchToCommand(cmd)
	return true
}
