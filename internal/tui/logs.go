package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	corev1 "k8s.io/api/core/v1"
)

type PodLogs struct {
	*widgets.Paragraph
	namespace string
	podName   string
}

func NewPodLogs(namespace, podName string) *PodLogs {
	return &PodLogs{
		Paragraph: widgets.NewParagraph(),
		namespace: namespace,
		podName:   podName,
	}
}

func (pl *PodLogs) Reload() error {
	client, err := kube.GetClient()
	if err != nil {
		return err
	}
	// TODO: enhance logs preview
	tail := int64(5)
	// TODO: select container when multiple containers in pod
	logs := client.CoreV1().Pods(pl.namespace).GetLogs(pl.podName, &corev1.PodLogOptions{
		Follow:    false,
		TailLines: &tail,
	}).Do()
	if logs.Error() != nil {
		pl.Text = logs.Error().Error()
		return nil
	}
	bytes, err := logs.Raw()
	if err != nil {
		pl.Text = err.Error()
	} else {
		pl.Text = string(bytes)
	}
	return nil
}

func (pl *PodLogs) OnEvent(event *termui.Event) bool {
	return false
}
