package commander

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceMap map[schema.GroupKind]*Resource

type ResourceProvider interface {
	Resources() (ResourceMap, error)
}

type Resource struct {
	Namespaced bool
	Resource   string
	Gk         schema.GroupKind
	Gvk        schema.GroupVersionKind
	Verbs      []string
}

func (r Resource) GroupVersion() schema.GroupVersion {
	return r.Gvk.GroupVersion()
}

func (r Resource) GroupVersionKind() schema.GroupVersionKind {
	return r.Gvk
}

func (r Resource) GroupVersionResource() schema.GroupVersionResource {
	return r.GroupVersion().WithResource(r.Resource)
}

func (r Resource) Scope() meta.RESTScope {
	if r.Namespaced {
		return meta.RESTScopeNamespace
	} else {
		return meta.RESTScopeRoot
	}
}

type ResourceContainer interface {
	NamespaceAccessor
	Status() StatusReporter
	Client() Client
	ResourceProvider() ResourceProvider
	CommandBuilder() CommandBuilder
	CommandExecutor() CommandExecutor
	ScreenHandler() ScreenHandler
}
