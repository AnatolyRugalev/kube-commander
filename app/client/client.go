package client

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"

	"github.com/AnatolyRugalev/kube-commander/commander"
)

func init() {
	scheme.Scheme.AddKnownTypeWithName(
		schema.GroupVersion{
			Group:   metav1.GroupName,
			Version: runtime.APIVersionInternal,
		}.WithKind("Table"),
		&metav1.Table{},
	)
}

func NewClient(config commander.Config) (*client, error) {
	cl := &client{
		config: config,
	}
	return cl, nil
}

type client struct {
	config  commander.Config
	timeout time.Duration
}

func (c client) Delete(ctx context.Context, resource *commander.Resource, namespace string, name string) error {
	req, err := c.NewRequest(resource)
	if err != nil {
		return err
	}
	req.
		Verb("DELETE").
		Name(name)
	if resource.Namespaced {
		req.Namespace(namespace)
	}
	res := req.Do(ctx)
	return res.Error()
}

func (c client) NewRequest(resource *commander.Resource) (*rest.Request, error) {
	restClient, err := c.rest(resource.GroupVersion())
	if err != nil {
		return nil, err
	}
	r := rest.NewRequest(restClient)
	r.Resource(resource.Resource)
	r.Timeout(time.Second * 5)
	return r, nil
}

func (c client) Resources() (commander.ResourceMap, error) {
	discoveryClient, err := c.config.Factory().ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	lists, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resources := make(commander.ResourceMap)

	for _, list := range lists {
		for _, res := range list.APIResources {
			gv, err := schema.ParseGroupVersion(list.GroupVersion)
			if err != nil {
				return nil, err
			}

			gk := schema.GroupKind{Group: gv.Group, Kind: res.Kind}
			resources[gk] = &commander.Resource{
				Namespaced: res.Namespaced,
				Resource:   res.Name,
				Gk:         gk,
				Gvk:        schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: res.Kind},
				Verbs:      res.Verbs,
			}
		}
	}

	return resources, nil
}

func (c client) Get(ctx context.Context, resource *commander.Resource, namespace string, name string, out runtime.Object) error {
	opts := metav1.GetOptions{}
	req, err := c.NewRequest(resource)
	if err != nil {
		return err
	}
	req.
		Verb("GET").
		VersionedParams(&opts, scheme.ParameterCodec).
		Name(name)
	if resource.Namespaced {
		req.Namespace(namespace)
	}
	err = req.Do(ctx).Into(out)
	if err != nil {
		return err
	}
	return nil
}

func (c client) ListAsTable(ctx context.Context, resource *commander.Resource, namespace string) (*metav1.Table, error) {
	table := metav1.Table{}
	err := c.List(ctx, resource, namespace, &table)
	if err != nil {
		return nil, err
	}
	return &table, nil
}

func (c client) List(ctx context.Context, resource *commander.Resource, namespace string, out runtime.Object) error {
	req, err := c.NewRequest(resource)
	if err != nil {
		return err
	}

	req.
		Verb("GET")
	switch out.(type) {
	case *metav1.Table:
		req.SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ","))
	}
	if resource.Namespaced {
		req.Namespace(namespace)
	}
	err = req.Do(ctx).Into(out)
	if err != nil {
		return err
	}
	return nil
}

func (c client) transformRequests(req *rest.Request) {
	req.SetHeader("Accept", strings.Join([]string{
		fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
		fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
		"application/json",
	}, ","))
}

func (c client) WatchAsTable(_ context.Context, r *commander.Resource, namespace string) (watch.Interface, error) {
	b := c.config.Factory().NewBuilder()
	b.WithScheme(scheme.Scheme)
	b.ResourceTypeOrNameArgs(false, r.Resource)
	if namespace == "" {
		b.AllNamespaces(true)
	} else {
		b.NamespaceParam(namespace)
	}
	b.SingleResourceType()
	b.SelectAllParam(true)
	b.NamespaceParam(namespace)
	b.Latest()
	b.TransformRequests(c.transformRequests)
	result := b.Do()
	obj, err := result.Object()
	if err != nil {
		return nil, err
	}
	rv, err := meta.NewAccessor().ResourceVersion(obj)
	if err != nil {
		return nil, err
	}
	return result.Watch(rv)
}

func (c client) rest(gv schema.GroupVersion) (*rest.RESTClient, error) {
	conf, err := c.config.Factory().ToRESTConfig()
	if err != nil {
		return nil, err
	}
	conf.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	conf.GroupVersion = &gv
	if gv.Group == "" {
		conf.APIPath = "/api"
	} else {
		conf.APIPath = "/apis"
	}
	return rest.RESTClientFor(conf)
}
