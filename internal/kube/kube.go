package kube

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
	"os/user"
	"strings"
)

type KubeClient struct {
	*kubernetes.Clientset
}

var config = &struct {
	Path      string `mapstructure:"kubeconfig"`
	Context   string `mapstructure:"context"`
	Namespace string `mapstructure:"namespace"`
}{}

var client *KubeClient

func init() {
	u, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	slash := string(os.PathSeparator)
	kubeconfig := strings.TrimRight(u.HomeDir, slash) + slash + ".kube" + slash + "config"
	cfg.AddPkg(&cfg.Pkg{
		Struct: config,
		PersistentFlags: cfg.FlagsDeclaration{
			"kubeconfig": {kubeconfig, "Kubernetes kubeconfig path", "KUBECONFIG"},
			"context":    {"", "Kubernetes context to use", "KUBECONTEXT"},
			"namespace":  {"", "Kubernetes context to use", "KUBENAMESPACE"},
		},
		Validate: initClient,
	})
}

func getClientConfig() (*rest.Config, error) {
	clientConfig := cmd.
		NewNonInteractiveDeferredLoadingClientConfig(
			&cmd.ClientConfigLoadingRules{ExplicitPath: config.Path},
			&cmd.ConfigOverrides{
				CurrentContext: config.Context,
			},
		)
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return nil, err
	}
	if config.Context == "" {
		// lock context if default context is being used
		config.Context = raw.CurrentContext
	}
	if config.Namespace == "" {
		// lock context if default context is being used
		config.Namespace, _, _ = clientConfig.Namespace()
	}
	return clientConfig.ClientConfig()
}

func GetNamespace() string {
	return config.Namespace
}

func GetClient() *KubeClient {
	return client
}

func initClient() error {
	c, err := getClientConfig()
	if err != nil {
		return err
	}
	clientSet, err := kubernetes.NewForConfig(c)
	if err != nil {
		return err
	}
	client = &KubeClient{clientSet}
	return nil
}
