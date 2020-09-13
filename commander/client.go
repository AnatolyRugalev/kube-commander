package commander

import (
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

type Client interface {
	NewRequest(resource *Resource) (*rest.Request, error)
	Get(ctx context.Context, resource *Resource, namespace string, name string, out runtime.Object) error
	Delete(ctx context.Context, resource *Resource, namespace string, name string) error
	List(ctx context.Context, resource *Resource, namespace string, out runtime.Object) error
	ListAsTable(ctx context.Context, resource *Resource, namespace string) (*metav1.Table, error)
	WatchAsTable(ctx context.Context, resource *Resource, namespace string) (watch.Interface, error)
}
