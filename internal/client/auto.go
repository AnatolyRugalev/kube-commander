package client

import (
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
)

type autoConfigProvider struct {
	cmd *cobra.Command
}

func (a autoConfigProvider) Context() string {
	return ""
}

func (a autoConfigProvider) Kubeconfig() string {
	return ""
}

func NewAutoConfigProvider() ConfigProvider {
	return &autoConfigProvider{}
}

func (a autoConfigProvider) ClientConfig() (*rest.Config, error) {
	rules := cmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &cmd.DefaultClientConfig
	clientConfig := cmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &cmd.ConfigOverrides{})
	return clientConfig.ClientConfig()
}
