package kube

import (
	"context"
	"testing"

	"k8s.io/client-go/rest"
)

// eventsTableJSON is a server-printed events Table as a real cluster answers a
// `kubectl get events` for one object: the kubectl columns LAST SEEN, TYPE,
// REASON, OBJECT, MESSAGE.
const eventsTableJSON = `{
  "kind": "Table",
  "apiVersion": "meta.k8s.io/v1",
  "columnDefinitions": [
    {"name": "LAST SEEN", "type": "string", "format": "", "description": "", "priority": 0},
    {"name": "TYPE", "type": "string", "format": "", "description": "", "priority": 0},
    {"name": "REASON", "type": "string", "format": "", "description": "", "priority": 0},
    {"name": "OBJECT", "type": "string", "format": "", "description": "", "priority": 0},
    {"name": "MESSAGE", "type": "string", "format": "", "description": "", "priority": 0}
  ],
  "rows": [
    {
      "cells": ["3m", "Warning", "BackOff", "pod/web-1", "Back-off restarting failed container"],
      "object": {"kind":"PartialObjectMetadata","apiVersion":"meta.k8s.io/v1","metadata":{"name":"web-1.abc","namespace":"web","uid":"ev-1"}}
    }
  ]
}`

func TestEventsFieldSelector(t *testing.T) {
	cases := []struct {
		name string
		ref  ObjectRef
		want string
	}{
		{"uid preferred", ObjectRef{Namespace: "web", Name: "web-1", UID: "uid-1"}, "involvedObject.uid=uid-1"},
		{"no uid falls back to name", ObjectRef{Namespace: "web", Name: "web-1"}, "involvedObject.name=web-1"},
	}
	for _, tc := range cases {
		if got := eventsFieldSelector(tc.ref); got != tc.want {
			t.Errorf("%s: eventsFieldSelector = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestGetEventsNamespaced(t *testing.T) {
	client := newTableRESTClient(eventsGVR.GroupVersion(), "/api/v1", eventsTableJSON)

	tbl, err := getEvents(context.Background(), client, ObjectRef{Namespace: "web", Name: "web-1", UID: "uid-1"})
	if err != nil {
		t.Fatalf("getEvents: %v", err)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(tbl.Rows))
	}

	// The request must negotiate server-side Table printing, be namespace-scoped
	// to the object's namespace, and filter to the object's own events by UID.
	if client.Req == nil {
		t.Fatal("no request recorded")
	}
	if got := client.Req.Header.Get("Accept"); got != tableAcceptHeader {
		t.Errorf("Accept = %q, want %q", got, tableAcceptHeader)
	}
	if got, want := client.Req.URL.Path, "/api/v1/namespaces/web/events"; got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if got, want := client.Req.URL.Query().Get("fieldSelector"), "involvedObject.uid=uid-1"; got != want {
		t.Errorf("fieldSelector = %q, want %q", got, want)
	}
}

func TestGetEventsClusterScopedRef(t *testing.T) {
	// A cluster-scoped object (a Node) has an empty namespace; an empty namespace
	// lists events across all namespaces, which is where a Node's events live.
	// Without a UID the filter degrades to the name.
	client := newTableRESTClient(eventsGVR.GroupVersion(), "/api/v1", eventsTableJSON)

	tbl, err := getEvents(context.Background(), client, ObjectRef{Name: "node-1"})
	if err != nil {
		t.Fatalf("getEvents: %v", err)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(tbl.Rows))
	}
	if got, want := client.Req.URL.Path, "/api/v1/events"; got != want {
		t.Errorf("path = %q, want %q (empty namespace lists across all)", got, want)
	}
	if got, want := client.Req.URL.Query().Get("fieldSelector"), "involvedObject.name=node-1"; got != want {
		t.Errorf("fieldSelector = %q, want %q", got, want)
	}
}

func TestEventsRejectsEmptyName(t *testing.T) {
	c, err := NewClients(&rest.Config{Host: "https://localhost:6443"})
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if _, err := c.Events(context.Background(), ObjectRef{}); err == nil {
		t.Fatal("Events with an empty name should error, not list unfiltered")
	}
}
