package client

import (
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	cmd "k8s.io/client-go/tools/clientcmd"
)

type defaultConfig struct {
	cmd *cobra.Command
}

func (a defaultConfig) Context() string {
	return ""
}

func (a defaultConfig) Kubeconfig() string {
	return ""
}

func NewDefaultConfig() *defaultConfig {
	return &defaultConfig{}
}

func (a defaultConfig) ClientConfig() (*rest.Config, error) {
	rules := cmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &cmd.DefaultClientConfig
	clientConfig := cmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &cmd.ConfigOverrides{})
	return clientConfig.ClientConfig()
}
