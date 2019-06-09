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

func (pt *PodsTable) Delete(idx int, row widgets.ListRow) error {
	return kube.GetClient().CoreV1().Pods(pt.namespace).Delete(row[0], metav1.NewDeleteOptions(0))
}

func (pt *PodsTable) DeleteDescription(idx int, row widgets.ListRow) string {
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

func (pt *PodsTable) OnExec(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
	pod, err := kube.GetClient().CoreV1().Pods(pt.namespace).Get(row[0], metav1.GetOptions{})
	if err != nil {
		ShowErrorDialog(err, nil)
		return true
	}
	if len(pod.Spec.Containers)+len(pod.Spec.InitContainers) > 1 {
		pt.ShowContainerSelection(pod, func(container string) {
			pt.execToPod(row, container)
		})
	} else {
		pt.execToPod(row, "")
	}
	return true
}

func (pt *PodsTable) OnLogs(handler widgets.ListTableHandler, idx int, row widgets.ListRow) bool {
	pod, err := kube.GetClient().CoreV1().Pods(pt.namespace).Get(row[0], metav1.GetOptions{})
	if err != nil {
		ShowErrorDialog(err, nil)
		return true
	}
	if len(pod.Spec.Containers)+len(pod.Spec.InitContainers) > 1 {
		pt.ShowContainerSelection(pod, func(container string) {
			pt.showLogs(pod, container)
		})
	} else {
		pt.showLogs(pod, "")
	}
	return true
}

func (pt *PodsTable) showLogs(pod *v1.Pod, container string) {
	var cmd string
	var running bool
	if container == "" {
		container = pod.Spec.Containers[0].Name
	}

	for _, s := range pod.Status.InitContainerStatuses {
		if s.Name == container {
			running = s.State.Running != nil
			break
		}
	}

	for _, s := range pod.Status.ContainerStatuses {
		if s.Name == container {
			running = s.State.Running != nil
			break
		}
	}

	if running {
		cmd = kube.Logs(pt.namespace, pod.Name, container, 1000, true)
	} else {
		cmd = kube.Viewer(kube.Logs(pt.namespace, pod.Name, container, 1000, false))
	}
	screen.SwitchToCommand(cmd)
}

func (pt *PodsTable) execToPod(row widgets.ListRow, container string) {
	screen.SwitchToCommand(kube.Exec(
		pt.namespace,
		row[0],
		container,
		"/bin/sh -- -c '/bin/bash || /bin/sh'",
	))
}

func (pt *PodsTable) ShowContainerSelection(pod *v1.Pod, onSelect func(container string)) {
	var rows []widgets.ListRow
	for _, container := range pod.Status.InitContainerStatuses {
		var state string
		if container.State.Running != nil {
			state = "Running"
		} else if container.State.Waiting != nil {
			state = "Waiting"
		} else if container.State.Terminated != nil {
			state = "Terminated"
		}
		rows = append(rows, widgets.ListRow{
			container.Name,
			state,
		})
	}
	for _, container := range pod.Status.ContainerStatuses {
		var state string
		if container.State.Running != nil {
			state = "Running"
		} else if container.State.Waiting != nil {
			state = "Waiting"
		} else if container.State.Terminated != nil {
			state = "Terminated"
		}
		rows = append(rows, widgets.ListRow{
			container.Name,
			state,
		})
	}
	handler := &ContainerSelectorHandler{
		pod:      pod,
		onSelect: onSelect,
	}
	menu := widgets.NewListTable(rows, handler, nil)
	menu.Title = "Select container"
	menu.IsContext = true
	width := 30
	height := len(rows) + 2
	y1 := screen.Rectangle.Max.Y/2 - height/2
	x1 := screen.Rectangle.Max.X/2 - width/2
	menu.SetRect(x1, y1, x1+width, y1+height)
	screen.setPopup(menu)
	screen.Focus(menu)
}

type ContainerSelectorHandler struct {
	pod      *v1.Pod
	onSelect func(container string)
}

func (csh *ContainerSelectorHandler) OnSelect(idx int, row widgets.ListRow) bool {
	screen.popFocus()
	screen.removePopup()
	container := row[0]
	csh.onSelect(container)
	return true
}
