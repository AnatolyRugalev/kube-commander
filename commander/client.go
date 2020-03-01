package commander

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

type ResourceMap map[string]*Resource

type ResourceProvider interface {
	PreferredResources() (ResourceMap, error)
}

type Client interface {
	NewRequest(resource *Resource) (*rest.Request, error)
	Get(resource *Resource, namespace string, name string, out runtime.Object) error
	List(resource *Resource, namespace string, out runtime.Object) error
	ListAsTable(resource *Resource, namespace string) (*metav1.Table, error)
}
