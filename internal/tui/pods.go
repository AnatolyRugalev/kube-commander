package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"k8s.io/api/core/v1"
	"time"
)

type PodsTable struct {
	*widgets.Table
	Namespace string
}

func (pt *PodsTable) OnEvent(event *termui.Event) bool {
	return false
}

func (pt *PodsTable) OnFocusIn() {
	_ = pt.Reload()
}

func (pt *PodsTable) OnFocusOut() {

}

func NewPodsTable(namespace string) *PodsTable {
	pt := &PodsTable{widgets.NewTable(), namespace}
	pt.Title = "Pods"
	pt.RowSeparator = false
	pt.resetRows()
	return pt
}

func (pt *PodsTable) resetRows() {
	pt.Rows = [][]string{
		pt.getTitleRow(),
	}
}

func (pt *PodsTable) getTitleRow() []string {
	return []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
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
		fmt.Sprintf("%d/%d", total, ready),
		string(pod.Status.Phase),
		fmt.Sprintf("%d", restarts),
		Age(pod.Status.StartTime.Time),
	}
}

func Age(startTime time.Time) string {
	// TODO: humanize
	return time.Since(startTime).Round(time.Second).String()
}

func (pt *PodsTable) Reload() error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	pods, err := client.GetPods(pt.Namespace)
	if err != nil {
		return err
	}
	pt.resetRows()
	for _, pod := range pods.Items {
		pt.Rows = append(pt.Rows, pt.newRow(pod))
	}
	return nil
}
