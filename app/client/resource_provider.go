package client

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	v1 "k8s.io/api/core/v1"
	"strings"
)

var coreResources = commander.ResourceMap{
	"Node":                  {Namespaced: false},
	"Namespace":             {Namespaced: false},
	"PersistentVolume":      {Namespaced: false},
	"Pod":                   {Namespaced: true},
	"ConfigMap":             {Namespaced: true},
	"Secret":                {Namespaced: true},
	"Service":               {Namespaced: true},
	"ServiceAccount":        {Namespaced: true},
	"PersistentVolumeClaim": {Namespaced: true},
}

func init() {
	// Prepare resource names
	for kind, res := range coreResources {
		resName := strings.ToLower(kind)
		if strings.HasSuffix(resName, "s") {
			resName += "es"
		} else {
			resName += "s"
		}
		res.Resource = resName
		res.Gvk = v1.SchemeGroupVersion.WithKind(kind)
	}
}

func CoreResources() commander.ResourceMap {
	return coreResources
}
