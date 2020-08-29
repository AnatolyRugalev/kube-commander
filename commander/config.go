package commander

type Config interface {
	Context() string
	Kubeconfig() string
	Namespace() string
}

type ConfigAccessor func() Config
