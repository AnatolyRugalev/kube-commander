package commander

import (
	"github.com/AnatolyRugalev/kube-commander/pb"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubectl/pkg/cmd/util"
)

type Config interface {
	Context() string
	Namespace() string
	Kubeconfig() string
	Factory() util.Factory
	Raw() api.Config
}

type ConfigAccessor func() Config

type ConfigUpdateFunc func(config *pb.Config)

type ConfigUpdater interface {
	UpdateConfig(updateFunc ConfigUpdateFunc) error
}

type Configurable interface {
	ConfigUpdated(config *pb.Config)
}
