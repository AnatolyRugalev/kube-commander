package client

import (
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
)

type cmdConfigProvider struct {
	kubeconfig string
	context    string
}

func (c cmdConfigProvider) Context() string {
	return c.context
}

func (c cmdConfigProvider) Kubeconfig() string {
	return c.kubeconfig
}

func NewCmdConfigProvider(kubeconfig string, context string) ConfigProvider {
	return &cmdConfigProvider{
		kubeconfig: kubeconfig,
		context:    context,
	}
}

func (c cmdConfigProvider) ClientConfig() (*rest.Config, error) {
	rules := cmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &cmd.DefaultClientConfig
	if c.kubeconfig != "" {
		rules.ExplicitPath = c.kubeconfig
	}
	clientConfig := cmd.
		NewNonInteractiveDeferredLoadingClientConfig(
			rules,
			&cmd.ConfigOverrides{
				CurrentContext:  c.context,
				ClusterDefaults: cmd.ClusterDefaults,
			},
		)
	return clientConfig.ClientConfig()
}
