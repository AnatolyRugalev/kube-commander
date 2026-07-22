package kube

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	restfake "k8s.io/client-go/rest/fake"
)

// A server-printed pods Table: two columns, two rows, each with embedded
// PartialObjectMetadata (the Table default IncludeObject=Metadata).
const podsTableJSON = `{
  "kind": "Table",
  "apiVersion": "meta.k8s.io/v1",
  "columnDefinitions": [
    {"name": "Name", "type": "string", "format": "name", "description": "Name of the resource", "priority": 0},
    {"name": "Status", "type": "string", "format": "", "description": "", "priority": 0},
    {"name": "IP", "type": "string", "format": "", "description": "", "priority": 1}
  ],
  "rows": [
    {
      "cells": ["nginx-abc", "Running", "10.0.0.1"],
      "object": {"kind":"PartialObjectMetadata","apiVersion":"meta.k8s.io/v1","metadata":{"name":"nginx-abc","namespace":"web","uid":"uid-1"}}
    },
    {
      "cells": ["redis-def", "Pending", "10.0.0.2"],
      "object": {"kind":"PartialObjectMetadata","apiVersion":"meta.k8s.io/v1","metadata":{"name":"redis-def","namespace":"web","uid":"uid-2"}}
    }
  ]
}`

func TestDecodeTable(t *testing.T) {
	tbl, err := decodeTable([]byte(podsTableJSON))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}

	if got, want := len(tbl.Columns), 3; got != want {
		t.Fatalf("columns = %d, want %d", got, want)
	}
	if got, want := tbl.Columns[0].Name, "Name"; got != want {
		t.Errorf("Columns[0].Name = %q, want %q", got, want)
	}
	if got, want := tbl.Columns[0].Format, "name"; got != want {
		t.Errorf("Columns[0].Format = %q, want %q", got, want)
	}
	if got, want := tbl.Columns[2].Priority, int32(1); got != want {
		t.Errorf("Columns[2].Priority = %d, want %d (hidden-first column)", got, want)
	}

	if got, want := len(tbl.Rows), 2; got != want {
		t.Fatalf("rows = %d, want %d", got, want)
	}
	r0 := tbl.Rows[0]
	if got, want := len(r0.Cells), 3; got != want {
		t.Fatalf("row0 cells = %d, want %d", got, want)
	}
	if got, want := r0.Cells[1], "Running"; got != want {
		t.Errorf("row0 cell[1] = %v, want %q", got, want)
	}
	if got := r0.Object; got.Name != "nginx-abc" || got.Namespace != "web" || got.UID != "uid-1" {
		t.Errorf("row0 Object = %+v, want {web nginx-abc uid-1}", got)
	}
	if got := tbl.Rows[1].Object.UID; got != "uid-2" {
		t.Errorf("row1 UID = %q, want uid-2", got)
	}
}

func TestDecodeTableRowWithoutObject(t *testing.T) {
	// A row missing/empty object metadata is kept with a zero ObjectRef, not
	// dropped (degrade, don't crash). A malformed object likewise degrades to a
	// zero ref without failing the whole decode.
	const j = `{
      "kind":"Table","apiVersion":"meta.k8s.io/v1",
      "columnDefinitions":[{"name":"Name","type":"string"}],
      "rows":[
        {"cells":["no-meta"]},
        {"cells":["bad-meta"],"object":{"metadata":"not-an-object"}}
      ]
    }`
	tbl, err := decodeTable([]byte(j))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	if got, want := len(tbl.Rows), 2; got != want {
		t.Fatalf("rows = %d, want %d (rows must be kept)", got, want)
	}
	if got := tbl.Rows[0].Object; got != (ObjectRef{}) {
		t.Errorf("row0 Object = %+v, want zero", got)
	}
	if got := tbl.Rows[1].Object; got != (ObjectRef{}) {
		t.Errorf("row1 Object = %+v, want zero (malformed metadata degrades)", got)
	}
	if got, want := tbl.Rows[0].Cells[0], "no-meta"; got != want {
		t.Errorf("row0 cell = %v, want %q", got, want)
	}
}

func TestDecodeTableInvalidJSON(t *testing.T) {
	if _, err := decodeTable([]byte("{not json")); err == nil {
		t.Fatal("decodeTable(invalid): want error, got nil")
	}
}

// newTableRESTClient returns a fake REST client that responds to any request
// with the given body and records the request it received.
func newTableRESTClient(gv schema.GroupVersion, apiPath, body string) *restfake.RESTClient {
	return &restfake.RESTClient{
		NegotiatedSerializer: scheme.Codecs,
		GroupVersion:         gv,
		VersionedAPIPath:     apiPath,
		Resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		},
	}
}

func TestGetTableNamespaced(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	client := newTableRESTClient(gvr.GroupVersion(), "/api/v1", podsTableJSON)

	tbl, err := getTable(context.Background(), client, gvr, true, "web", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("getTable: %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(tbl.Rows))
	}

	// The request must negotiate server-side Table printing and be namespace-scoped.
	if client.Req == nil {
		t.Fatal("no request recorded")
	}
	if got := client.Req.Header.Get("Accept"); got != tableAcceptHeader {
		t.Errorf("Accept = %q, want %q", got, tableAcceptHeader)
	}
	if got, want := client.Req.URL.Path, "/api/v1/namespaces/web/pods"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestGetTableClusterScoped(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	client := newTableRESTClient(gvr.GroupVersion(), "/api/v1", podsTableJSON)

	if _, err := getTable(context.Background(), client, gvr, false, "web", metav1.ListOptions{}); err != nil {
		t.Fatalf("getTable: %v", err)
	}
	// A cluster-scoped resource must not carry a namespace segment even when one
	// is passed in.
	if got, want := client.Req.URL.Path, "/api/v1/nodes"; got != want {
		t.Errorf("path = %q, want %q (namespace must be dropped)", got, want)
	}
}

func TestGetTableNonBuiltinGroup(t *testing.T) {
	// Regression for the CRD list/watch bug: a resource whose GroupVersion is not
	// in the built-in clientset scheme (e.g. a CRD like gateway.networking.k8s.io
	// or traefik.io) must still list. The bug was encoding VersionedParams with
	// scheme.ParameterCodec (built-in scheme only), which cannot convert
	// metav1.ListOptions to an arbitrary CRD GroupVersion and fails with
	// "v1.ListOptions is not suitable for converting to ...". Encoding with
	// metav1.ParameterCodec converts params for any GroupVersion, so this passes.
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	client := newTableRESTClient(gvr.GroupVersion(), "/apis/example.com/v1", podsTableJSON)

	// A non-empty ListOptions ensures params are actually encoded onto the request.
	opts := metav1.ListOptions{LabelSelector: "app=demo", ResourceVersion: "42"}
	tbl, err := getTable(context.Background(), client, gvr, true, "web", opts)
	if err != nil {
		t.Fatalf("getTable(non-built-in group): %v", err)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(tbl.Rows))
	}
	if client.Req == nil {
		t.Fatal("no request recorded")
	}
	if got, want := client.Req.URL.Path, "/apis/example.com/v1/namespaces/web/widgets"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	// The list controls must have been encoded into the query, proving params
	// converted for the non-built-in GroupVersion rather than erroring.
	if got := client.Req.URL.Query().Get("labelSelector"); got != "app=demo" {
		t.Errorf("labelSelector = %q, want %q", got, "app=demo")
	}
}

func TestRestClientForGV(t *testing.T) {
	c, err := NewClients(&rest.Config{Host: "https://localhost:6443"})
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	cases := []struct {
		gv      schema.GroupVersion
		wantAPI string
	}{
		{schema.GroupVersion{Group: "", Version: "v1"}, "/api"},
		{schema.GroupVersion{Group: "apps", Version: "v1"}, "/apis"},
	}
	for _, tc := range cases {
		rc, err := c.restClientForGV(tc.gv)
		if err != nil {
			t.Fatalf("restClientForGV(%s): %v", tc.gv, err)
		}
		if rc == nil {
			t.Fatalf("restClientForGV(%s): nil client", tc.gv)
		}
		if got, want := rc.APIVersion(), tc.gv; got != want {
			t.Errorf("APIVersion = %v, want %v", got, want)
		}
	}
}
