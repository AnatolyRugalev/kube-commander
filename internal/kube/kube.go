package kube

import (
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
	Timeout            int    `mapstructure:"timeout"`
}{}

var restConfig *rest.Config

func GetClientConfig() (*rest.Config, error) {
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
		// lock namespace if default namespace is being used
		config.Namespace, _, _ = clientConfig.Namespace()
	}
	return clientConfig.ClientConfig()
}

func GetTimeout() int {
	return config.Timeout
}

func Context() string {
	return config.Context
}

func GetClient() *KubeClient {
	return nil
}
