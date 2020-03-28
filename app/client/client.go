package client

import (
	"fmt"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/describe"
	"k8s.io/kubectl/pkg/describe/versioned"
	"k8s.io/kubectl/pkg/scheme"
	"strings"
	"time"
)

const AllNamespaces = "All namespaces"

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
	c, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}
	c.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	r, err := rest.UnversionedRESTClientFor(c)
	if err != nil {
		return nil, err
	}
	cl := &client{
		config:     config,
		restConfig: c,
		restClient: r,
	}
	return cl, nil
}

type client struct {
	config     commander.Config
	restConfig *rest.Config
	restClient *rest.RESTClient
	timeout    time.Duration

	resources commander.ResourceMap
}

func (c client) NewRequest(resource *commander.Resource) (*rest.Request, error) {
	restClient, err := c.rest(resource.GroupVersion())
	if err != nil {
		return nil, err
	}
	r := rest.NewRequest(restClient)
	timeout := c.restConfig.Timeout
	if timeout == 0 {
		timeout = time.Second * 5
	}
	r.Resource(resource.Resource)
	r.Timeout(timeout)
	return r, nil
}

func (c client) Resources() (commander.ResourceMap, error) {
	if c.resources == nil {
		lists, err := discovery.NewDiscoveryClient(c.restClient).ServerPreferredResources()
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

				resources[res.Kind] = &commander.Resource{
					Group:      gv.Group,
					Version:    gv.Version,
					Namespaced: res.Namespaced,
					Resource:   res.Name,
					Kind:       res.Kind,
				}
			}
		}
		c.resources = resources
	}
	return c.resources, nil
}

func (c client) Get(resource *commander.Resource, namespace string, name string, out runtime.Object) error {
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
	err = req.Do().Into(out)
	if err != nil {
		return err
	}
	return nil
}

func (c client) ListAsTable(resource *commander.Resource, namespace string) (*metav1.Table, error) {
	table := metav1.Table{}
	err := c.List(resource, namespace, &table)
	if err != nil {
		return nil, err
	}
	return &table, nil
}

func (c client) List(resource *commander.Resource, namespace string, out runtime.Object) error {
	opts := metav1.ListOptions{}
	req, err := c.NewRequest(resource)
	if err != nil {
		return err
	}

	req.
		Verb("GET").
		VersionedParams(&opts, scheme.ParameterCodec)
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
	err = req.Do().Into(out)
	if err != nil {
		return err
	}
	return nil
}

func (c client) WatchAsTable(resource *commander.Resource, namespace string) (watch.Interface, error) {
	opts := metav1.ListOptions{
		Watch: true,
	}
	req, err := c.NewRequest(resource)
	if err != nil {
		return nil, err
	}

	req.
		Verb("GET").
		VersionedParams(&opts, scheme.ParameterCodec).
		SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ","))
	if resource.Namespaced {
		req.Namespace(namespace)
	}
	return req.Watch()
}

func (c client) rest(gv schema.GroupVersion) (*rest.RESTClient, error) {
	conf := *c.restConfig
	conf.GroupVersion = &gv
	if gv.Group == "" {
		conf.APIPath = "/api"
	} else {
		conf.APIPath = "/apis"
	}
	return rest.RESTClientFor(&conf)
}

func (c client) DescribeApi(resource *commander.Resource, namespace string, name string) (string, error) {
	mapping := meta.RESTMapping{
		GroupVersionKind: resource.GroupVersionKind(),
		Resource:         resource.GroupVersionResource(),
		Scope:            resource.Scope(),
	}
	descr, err := c.describer(&mapping)
	if err != nil {
		return "", err
	}
	return descr.Describe(namespace, name, describe.DescriberSettings{
		ShowEvents: true,
	})
}

func (c client) describer(mapping *meta.RESTMapping) (describe.Describer, error) {
	// try to get a describer
	if describer, ok := versioned.DescriberFor(mapping.GroupVersionKind.GroupKind(), c.restConfig); ok {
		return describer, nil
	}
	// if this is a kind we don't have a describer for yet, go generic if possible
	if genericDescriber, ok := versioned.GenericDescriberFor(mapping, c.restConfig); ok {
		return genericDescriber, nil
	}
	// otherwise return an unregistered error
	return nil, fmt.Errorf("no description has been implemented for %s", mapping.GroupVersionKind.String())
}
