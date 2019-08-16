package kube

import (
	"github.com/AnatolyRugalev/kube-commander/internal/cfg"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
)

type KubeClient struct {
	*kubernetes.Clientset
}

var config = &struct {
	ExplicitConfigPath string `mapstructure:"kubeconfig"`
	Context            string `mapstructure:"context"`
	Namespace          string `mapstructure:"namespace"`
}{}

var client *KubeClient

func init() {
	cfg.AddPkg(&cfg.Pkg{
		Struct: config,
		PersistentFlags: cfg.FlagsDeclaration{
			"kubeconfig": {
				"",
				"Kubernetes kubeconfig path",
				"",
			},
			"context":   {"", "Kubernetes context to use", "KUBECONTEXT"},
			"namespace": {"", "Kubernetes context to use", "KUBENAMESPACE"},
		},
	})
}

func getClientConfig() (*rest.Config, error) {
	rules := cmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &cmd.DefaultClientConfig
	if config.ExplicitConfigPath != "" {
		rules.ExplicitPath = config.ExplicitConfigPath
	} else {
		config.ExplicitConfigPath = rules.GetDefaultFilename()
	}
	clientConfig := cmd.
		NewNonInteractiveDeferredLoadingClientConfig(
			rules,
			&cmd.ConfigOverrides{
				CurrentContext:  config.Context,
				ClusterDefaults: cmd.ClusterDefaults,
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

func Context() string {
	return config.Context
}

func GetClient() *KubeClient {
	return client
}

func InitClient() error {
	c, err := getClientConfig()
	if err != nil {
		return err
	}
	clientSet, err := kubernetes.NewForConfig(c)
	if err != nil {
		return err
	}
	client = &KubeClient{clientSet}
	if _, err := client.ServerVersion(); err != nil {
		return err
	}
	return nil
}
