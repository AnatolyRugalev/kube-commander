package commander

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
