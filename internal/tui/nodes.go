package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/widgets"
	"github.com/gizak/termui/v3"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"strings"
)

type NodesTable struct {
}

func (nt *NodesTable) OnEvent(event *termui.Event, item []string) bool {
	switch event.ID {
	case "d":
		// TODO: refactor
		cmd := exec.Command("/bin/bash", "-c", "kubectl describe node "+item[0]+" | less")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		screen.Close()
		cmd.Run()
		screen.Reinit()
		return true
	case "e":
		// TODO: refactor
		cmd := exec.Command("/bin/bash", "-c", "kubectl edit node "+item[0]+"")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		screen.Close()
		cmd.Run()
		screen.Reinit()
		return true
	}
	return false
}

func NewNodesTable() *widgets.ListTable {
	lt := widgets.NewListTable(&NodesTable{})
	lt.Title = "Nodes"
	return lt
}

func (nt *NodesTable) GetHeaderRow() []string {
	return []string{"NAME", "STATUS", "ROLES", "AGE", "VERSION"}
}

func (nt *NodesTable) LoadData() ([][]string, error) {
	client, err := kube.GetClient()
	if err != nil {
		return nil, err
	}
	namespaces, err := client.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, ns := range namespaces.Items {
		rows = append(rows, nt.newRow(ns))
	}
	return rows, nil
}

func (nt *NodesTable) newRow(n v1.Node) []string {
	var roles []string
	for l := range n.Labels {
		if strings.HasPrefix(l, "node-role.kubernetes.io/") {
			roles = append(roles, l[24:])
		}
	}
	if len(roles) == 0 {
		roles = []string{"<none>"}
	}

	var conditions []string
	for _, c := range n.Status.Conditions {
		if c.Status == v1.ConditionTrue {
			conditions = append(conditions, string(c.Type))
		}
	}

	return []string{
		n.Name,
		strings.Join(conditions, ","),
		strings.Join(roles, ","),
		Age(n.CreationTimestamp.Time),
		n.Status.NodeInfo.KubeletVersion,
	}
}
