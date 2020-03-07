package commander

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceMap map[string]*Resource

type ResourceProvider interface {
	Resources() (ResourceMap, error)
}

type Resource struct {
	Namespaced bool
	Group      string
	Version    string
	Resource   string
	Kind       string
}

func (r Resource) GroupVersion() schema.GroupVersion {
	return schema.GroupVersion{Group: r.Group, Version: r.Version}
}

func (r Resource) GroupVersionKind() schema.GroupVersionKind {
	return r.GroupVersion().WithKind(r.Kind)
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
	ErrorHandler
	Client() Client
	ResourceProvider() ResourceProvider
	CommandBuilder() CommandBuilder
	CommandExecutor() CommandExecutor
	ScreenUpdater() ScreenUpdater
}
