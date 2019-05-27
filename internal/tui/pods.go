package tui

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodsTable struct {
	Namespace string
}

func NewPodsTable(namespace string) *ListTable {
	lt := NewListTable(&PodsTable{
		Namespace: namespace,
	})
	lt.Title = "Pods <" + namespace + ">"
	return lt
}

func (pt *PodsTable) getTitleRow() []string {
	return []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"}
}

func (pt *PodsTable) loadData() ([][]string, error) {
	client, err := kube.GetClient()
	if err != nil {
		return nil, err
	}
	pods, err := client.CoreV1().Pods(pt.Namespace).List(metav1.ListOptions{})
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
		fmt.Sprintf("%d/%d", total, ready),
		string(pod.Status.Phase),
		fmt.Sprintf("%d", restarts),
		Age(pod.CreationTimestamp.Time),
	}
}

func (pt *PodsTable) OnDelete(item []string) error {
	name := item[0]
	client, err := kube.GetClient()
	if err != nil {
		return err
	}

	err = client.CoreV1().Pods(pt.Namespace).Delete(name, metav1.NewDeleteOptions(0))
	if err != nil {
		return err
	}

	return nil
}

func (pt *PodsTable) DeleteDialogText(item []string) string {
	return "Are you sure you want to delete pod " + item[0] + "?"
}

func (pt *PodsTable) Delete(name string) error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}

	err = client.CoreV1().Pods(pt.Namespace).Delete(name, metav1.NewDeleteOptions(0))
	if err != nil {
		return err
	}

	return nil
}
