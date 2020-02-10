package client

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"strings"
	"testing"
)

func TestGetResourceList(t *testing.T) {
	c, err := NewClient(NewAutoConfigProvider())
	if err != nil {
		t.Fatal(err)
	}
	lists, err := c.DiscoveryClient().ServerPreferredResources()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) == 0 {
		t.Fail()
	}
}

func TestGVRs(t *testing.T) {
	c, err := NewClient(NewAutoConfigProvider())
	if err != nil {
		t.Fatal(err)
	}
	gvrs, err := c.PreferredGroupVersionResources()
	if err != nil {
		t.Fatal(err)
	}
	if len(gvrs) == 0 {
		t.Fail()
	}
}

func TestTableOfPods(t *testing.T) {
	c, err := NewClient(NewAutoConfigProvider())
	if err != nil {
		t.Fatal(err)
	}
	rest, err := c.REST(&schema.GroupVersion{
		Group:   "",
		Version: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	table := metav1.Table{}
	err = rest.Get().
		Resource("pods").
		VersionedParams(&metav1.ListOptions{}, scheme.ParameterCodec).
		SetHeader("Accept", strings.Join([]string{
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
			fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1beta1.SchemeGroupVersion.Version, metav1beta1.GroupName),
			"application/json",
		}, ",")).
		Namespace("").
		Do().
		Into(&table)
	if err != nil {
		t.Fatal(err)
	}

	for i := range table.Rows {
		row := &table.Rows[i]
		if row.Object.Raw == nil || row.Object.Object != nil {
			continue
		}
		converted, err := runtime.Decode(unstructured.UnstructuredJSONScheme, row.Object.Raw)
		if err != nil {
			t.Fatal(err)
		}
		row.Object.Object = converted
	}

	if len(table.Rows) == 0 {
		t.Fail()
	}
}
