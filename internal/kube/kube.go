package kube

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
)

type KubeClient struct {
	*kubernetes.Clientset
}

var config = &struct {
	Path    string `mapstructure:"kube-config"`
	Context string `mapstructure:"kube-context"`
}{}

func init() {
	home := strings.TrimRight(os.Getenv("HOME"), "/")
	cfg.AddPkg(&cfg.Pkg{
		Struct: config,
		PersistentFlags: cfg.FlagsDeclaration{
			"kube-config":  {home + "/.kube/config", "Kubernetes kubeconfig path", "KUBECONFIG"},
			"kube-context": {"", "Kubernetes context to use", "KUBECONTEXT"},
		},
	})
}

func GetClient() (*KubeClient, error) {
	c, err := clientcmd.BuildConfigFromFlags("", config.Path)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return &KubeClient{clientset}, nil
}

func (k *KubeClient) GetPods(namespace string) (*v1.PodList, error) {
	return k.CoreV1().Pods(namespace).List(metav1.ListOptions{})
}
