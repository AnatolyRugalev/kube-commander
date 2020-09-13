package client

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/cmd/util"
)

type config struct {
	factory util.Factory
	raw     api.Config

	context    string
	namespace  string
	kubeconfig string
}

func NewConfig(kubeconfig string, context string, namespace string, timeout string) (*config, error) {
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	if context != "" {
		kubeConfigFlags.Context = &context
	}
	if namespace != "" {
		kubeConfigFlags.Namespace = &namespace
	}
	if kubeconfig != "" {
		kubeConfigFlags.KubeConfig = &kubeconfig
	}
	kubeConfigFlags.Timeout = &timeout
	f := util.NewFactory(kubeConfigFlags)
	loader := f.ToRawKubeConfigLoader()
	if kubeconfig == "" {
		kubeconfig = loader.ConfigAccess().GetDefaultFilename()
	}
	cc, err := loader.RawConfig()
	if err != nil {
		return nil, err
	}
	namespace, _, err = loader.Namespace()
	if err != nil {
		return nil, err
	}
	if context == "" {
		context = cc.CurrentContext
	}
	return &config{
		factory:    f,
		raw:        cc,
		context:    context,
		namespace:  namespace,
		kubeconfig: kubeconfig,
	}, nil
}

func (c *config) Context() string {
	return c.context
}

func (c *config) Namespace() string {
	return c.namespace
}

func (c *config) Kubeconfig() string {
	return c.kubeconfig
}

func (c *config) Raw() api.Config {
	return c.raw
}

func (c *config) Factory() util.Factory {
	return c.factory
}
