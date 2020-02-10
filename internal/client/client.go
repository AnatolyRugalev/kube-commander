package client

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/describe"
	"k8s.io/kubectl/pkg/describe/versioned"
	"k8s.io/kubectl/pkg/util/openapi"
	"strings"
	"time"
)

const AllNamespaces = "All namespaces"

type ConfigProvider interface {
	ClientConfig() (*rest.Config, error)
	Context() string
	Kubeconfig() string
}

type Client interface {
	DiscoveryClient() *discovery.DiscoveryClient
	SupportedApiResources() ([]metav1.APIResource, error)
	OpenAPIResources() openapi.Resources
	Columns(gvk schema.GroupVersionKind) (string, bool)
	AllColumns() (map[schema.GroupVersionKind]string, error)
	REST(gv *schema.GroupVersion) (*rest.RESTClient, error)
	PreferredGroupVersionResources() (ResourceMap, error)
	NewRequest(gv *schema.GroupVersion) (*rest.Request, error)
	LoadResourceToTable(resource *Resource, namespace string) (*metav1.Table, error)

	Describe(namespace string, resType string, resName string) string
	Edit(namespace string, resType string, resName string) string
	PortForward(namespace string, pod string, port string) string
	Exec(namespace string, pod string, container string, command string) string
	Logs(namespace string, pod string, container string, tail int, follow bool) string
	Viewer(command string) string
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

type ResourceMap map[string]*Resource

func NewClient(provider ConfigProvider) (Client, error) {
	c, err := provider.ClientConfig()
	if err != nil {
		return nil, err
	}
	c.NegotiatedSerializer = serializer.NewCodecFactory(runtime.NewScheme())
	r, err := rest.UnversionedRESTClientFor(c)
	if err != nil {
		return nil, err
	}
	cl := &client{
		provider: provider,
		config:   c,
		rest:     r,
	}
	getter := openapi.NewOpenAPIGetter(cl.DiscoveryClient())
	resources, err := getter.Get()
	if err != nil {
		return nil, err
	}
	cl.resources = resources
	return cl, nil
}

type client struct {
	provider  ConfigProvider
	config    *rest.Config
	rest      *rest.RESTClient
	resources openapi.Resources
	timeout   time.Duration
}

func (c client) NewRequest(gv *schema.GroupVersion) (*rest.Request, error) {
	restClient, err := c.REST(gv)
	if err != nil {
		return nil, err
	}
	r := rest.NewRequest(restClient)
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = time.Second * 5
	}
	r.Timeout(timeout)
	return r, nil
}

func (c client) DiscoveryClient() *discovery.DiscoveryClient {
	return discovery.NewDiscoveryClient(c.rest)
}

func (c client) PreferredGroupVersionResources() (ResourceMap, error) {
	// TODO: caching
	lists, err := c.DiscoveryClient().ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	resources := make(ResourceMap)

	for _, list := range lists {
		for _, res := range list.APIResources {
			gv, err := schema.ParseGroupVersion(list.GroupVersion)
			if err != nil {
				return nil, err
			}

			resources[res.Kind] = &Resource{
				Group:      gv.Group,
				Version:    gv.Version,
				Namespaced: res.Namespaced,
				Resource:   res.Name,
				Kind:       res.Kind,
			}
		}
	}

	return resources, nil
}

func (c client) SupportedApiResources() ([]metav1.APIResource, error) {
	lists, err := c.DiscoveryClient().ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	var resources []metav1.APIResource
	for _, list := range lists {
		for _, res := range list.APIResources {
			supported := false
			for _, verb := range res.Verbs {
				if verb == "get" {
					supported = true
					break
				}
			}
			if supported {
				resources = append(resources, res)
			}
		}
	}
	return resources, nil
}

func (c client) OpenAPIResources() openapi.Resources {
	return c.resources
}

func (c client) Columns(gvk schema.GroupVersionKind) (string, bool) {
	resource := c.resources.LookupResource(gvk)
	if resource == nil {
		return "", false
	}
	return openapi.GetPrintColumns(resource.GetExtensions())
}

func (c client) AllColumns() (map[schema.GroupVersionKind]string, error) {
	lists, err := c.DiscoveryClient().ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	m := make(map[schema.GroupVersionKind]string)
	for _, list := range lists {
		for _, res := range list.APIResources {
			gvk := schema.FromAPIVersionAndKind(list.GroupVersion, res.Kind)
			columns, ok := c.Columns(gvk)
			if ok {
				m[gvk] = columns
			}
		}
	}
	return m, nil
}

func (c client) REST(gv *schema.GroupVersion) (*rest.RESTClient, error) {
	conf := *c.config
	conf.GroupVersion = gv
	if gv.Group == "" {
		conf.APIPath = "/api"
	} else {
		conf.APIPath = "/apis"
	}
	return rest.RESTClientFor(&conf)
}

func (c client) DescribeApi(resource *Resource, namespace string, name string) (string, error) {
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
	if describer, ok := versioned.DescriberFor(mapping.GroupVersionKind.GroupKind(), c.config); ok {
		return describer, nil
	}
	// if this is a kind we don't have a describer for yet, go generic if possible
	if genericDescriber, ok := versioned.GenericDescriberFor(mapping, c.config); ok {
		return genericDescriber, nil
	}
	// otherwise return an unregistered error
	return nil, fmt.Errorf("no description has been implemented for %s", mapping.GroupVersionKind.String())
}

func (c client) LoadResourceToTable(resource *Resource, namespace string) (*metav1.Table, error) {
	opts := metav1.ListOptions{}
	gv := resource.GroupVersion()
	req, err := c.NewRequest(&gv)
	if err != nil {
		return nil, err
	}

	table := metav1.Table{}
	req.
		Verb("GET").
		Resource(resource.Resource).
		VersionedParams(&opts, scheme.ParameterCodec).
		SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ",")).
		Namespace(namespace)
	err = req.Do().Into(&table)
	if err != nil {
		return nil, err
	}
	return &table, nil
}
