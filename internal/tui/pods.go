package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	ui "github.com/gizak/termui/v3"
	"k8s.io/api/core/v1"
	"time"
)

type PodsTable struct {
	*ListTable
	Namespace string
}

func (pt *PodsTable) OnFocusIn() {
	pt.ListTable.OnFocusIn()
}

func NewPodsTable(namespace string) *PodsTable {
	pt := &PodsTable{NewListTable(), namespace}
	pt.Title = "Pods"
	pt.RowSeparator = false
	pt.SelectedRowStyle = ui.NewStyle(ui.ColorYellow)
	pt.RowStyle = ui.NewStyle(ui.ColorWhite)
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

func (pt *PodsTable) Reload(errChan chan<- error) {
	client, err := kube.GetClient()
	if err != nil {
		errChan <- err
		return
	}
	pods, err := client.GetPods(pt.Namespace)
	if err != nil {
		errChan <- err
		return
	}
	pt.resetRows()
	for _, pod := range pods.Items {
		pt.Rows = append(pt.Rows, pt.newRow(pod))
	}
	close(errChan)
}
