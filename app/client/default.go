package client

type defaultConfig struct {
	kubeconfig string
	namespace  string
	context    string
}

func (d *defaultConfig) Context() string {
	return d.context
}

func (d *defaultConfig) Kubeconfig() string {
	return d.kubeconfig
}

func (d *defaultConfig) Namespace() string {
	return d.namespace
}

func NewDefaultConfig(kubeconfig string, context string, namespace string) *defaultConfig {
	return &defaultConfig{
		kubeconfig: kubeconfig,
		context:    context,
		namespace:  namespace,
	}
}
