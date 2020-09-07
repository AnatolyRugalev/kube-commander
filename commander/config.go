package commander

import "github.com/AnatolyRugalev/kube-commander/pb"

type Config interface {
	Context() string
	Kubeconfig() string
	Namespace() string
}

type ConfigAccessor func() Config

type ConfigUpdateFunc func(config *pb.Config)

type ConfigUpdater interface {
	UpdateConfig(updateFunc ConfigUpdateFunc) error
}

type Configurable interface {
	ConfigUpdated(config *pb.Config)
}
