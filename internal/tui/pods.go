package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodsTable struct {
	*ListTable
	Namespace string
}

func NewPodsTable(namespace string) *PodsTable {
	pt := &PodsTable{NewListTable(), namespace}
	pt.Title = "Pods <" + namespace + ">"
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
		Age(pod.CreationTimestamp.Time),
	}
}

func (pt *PodsTable) Reload() error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	pods, err := client.CoreV1().Pods(pt.Namespace).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	pt.resetRows()
	for _, pod := range pods.Items {
		pt.Rows = append(pt.Rows, pt.newRow(pod))
	}
	return nil
}
