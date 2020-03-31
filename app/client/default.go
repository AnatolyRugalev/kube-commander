package client

import (
	"fmt"
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type defaultConfig struct {
	kubeconfig string
	namespace  string
	context    string
}

func (d defaultConfig) Context() string {
	return d.context
}

func (d defaultConfig) Kubeconfig() string {
	return d.kubeconfig
}

func NewDefaultConfig(kubeconfig string, context string, namespace string) *defaultConfig {
	return &defaultConfig{
		kubeconfig: kubeconfig,
		context:    context,
		namespace:  namespace,
	}
}

func (d *defaultConfig) ClientConfig() (*rest.Config, error) {
	rules := cmd.NewDefaultClientConfigLoadingRules()
	config, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}
	if d.context == "" {
		d.context = config.CurrentContext
	}
	if ctx, ok := config.Contexts[config.CurrentContext]; ok && d.namespace == "" {
		d.namespace = ctx.Namespace
	}
	if d.namespace == "" {
		d.namespace = "default"
	}
	clientConfig := cmd.NewNonInteractiveClientConfig(*config, d.Context(), &cmd.ConfigOverrides{
		Context: clientcmdapi.Context{
			Namespace: d.namespace,
		},
	}, rules)
	return clientConfig.ClientConfig()
}
