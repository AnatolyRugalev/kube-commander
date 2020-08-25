package client

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	v1 "k8s.io/api/core/v1"
	"strings"
)

var coreResources = commander.ResourceMap{
	{Kind: "Node"}:                  {Namespaced: false},
	{Kind: "Namespace"}:             {Namespaced: false},
	{Kind: "PersistentVolume"}:      {Namespaced: false},
	{Kind: "Pod"}:                   {Namespaced: true},
	{Kind: "ConfigMap"}:             {Namespaced: true},
	{Kind: "Secret"}:                {Namespaced: true},
	{Kind: "Service"}:               {Namespaced: true},
	{Kind: "ServiceAccount"}:        {Namespaced: true},
	{Kind: "PersistentVolumeClaim"}: {Namespaced: true},
}

func init() {
	// TODO: support non-core stable APIs
	// Prepare resource names
	for gk, res := range coreResources {
		resName := strings.ToLower(gk.Kind)
		if strings.HasSuffix(resName, "s") {
			resName += "es"
		} else {
			resName += "s"
		}
		res.Resource = resName
		res.Gvk = v1.SchemeGroupVersion.WithKind(gk.Kind)
	}
}

func CoreResources() commander.ResourceMap {
	return coreResources
}
