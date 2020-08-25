package client

import (
	"github.com/AnatolyRugalev/kube-commander/commander"
	apps "k8s.io/api/apps/v1"
	batch "k8s.io/api/batch/v1"
	batchBeta "k8s.io/api/batch/v1beta1"
	core "k8s.io/api/core/v1"
	storageBeta "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
)

var inititalResources = map[schema.GroupVersion]map[string]bool{
	core.SchemeGroupVersion: {
		"Node":                  false,
		"Namespace":             false,
		"PersistentVolume":      false,
		"Pod":                   true,
		"ConfigMap":             true,
		"Secret":                true,
		"Service":               true,
		"ServiceAccount":        true,
		"PersistentVolumeClaim": true,
	},
	apps.SchemeGroupVersion: {
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
		"ReplicaSet":  true,
	},
	batch.SchemeGroupVersion: {
		"Job": true,
	},
	batchBeta.SchemeGroupVersion: {
		"CronJob": true,
	},
	storageBeta.SchemeGroupVersion: {
		"StorageClass": false,
	},
}

var coreResources = commander.ResourceMap{}

func init() {
	// Prepare resource names
	for gv, resMap := range inititalResources {
		for kind, namespaced := range resMap {
			resName := strings.ToLower(kind)
			if strings.HasSuffix(resName, "s") {
				resName += "es"
			} else {
				resName += "s"
			}
			res := commander.Resource{
				Namespaced: namespaced,
				Resource:   resName,
				Gvk:        gv.WithKind(kind),
			}
			coreResources[kind] = &res
		}
	}
}

func CoreResources() commander.ResourceMap {
	return coreResources
}
