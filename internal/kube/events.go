package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// eventsGVR is the core/v1 events resource — the one kind every object's events
// live on, whatever the object's own kind (a Pod's events, a PVC's events and a
// Node's events are all core Events pointing at the object by involvedObject).
var eventsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

// Events lists the events for the object a table row references (STORY-06f), as
// the server-printed Table — the `kubectl get events` columns (LAST SEEN, TYPE,
// REASON, OBJECT, MESSAGE) — so the "why is this red" surface renders the same
// columns kubectl shows and needs no column knowledge of its own (D33). The list
// is filtered to the object's own events by the involvedObject field selector.
//
// Events are namespaced, so the list is scoped to ref.Namespace. A cluster-scoped
// object (a Node, say) has an empty namespace, and an empty namespace lists across
// all namespaces — exactly the search kubectl describe runs for a cluster-scoped
// object. An empty name is rejected rather than listed unfiltered.
func (c *Clients) Events(ctx context.Context, ref ObjectRef) (*Table, error) {
	if ref.Name == "" {
		return nil, fmt.Errorf("kube: listing events: empty object name")
	}
	client, err := c.restClientForGV(eventsGVR.GroupVersion())
	if err != nil {
		return nil, err
	}
	return getEvents(ctx, client, ref)
}

// getEvents is the injectable core of Events: it lists the events Table for ref
// through the given REST client (real or fake), so the request/decode path is
// exercised hermetically (D18).
func getEvents(ctx context.Context, client rest.Interface, ref ObjectRef) (*Table, error) {
	opts := metav1.ListOptions{FieldSelector: eventsFieldSelector(ref)}
	return getTable(ctx, client, eventsGVR, true, ref.Namespace, opts)
}

// eventsFieldSelector narrows a namespace's events to one object's. The UID is
// the precise key — the same one kubectl describe's own SearchEvents filters on
// — and a row that lost its metadata (the degraded row of principle 3) falls
// back to the name, which can over-match (events for a same-named object of
// another kind) but is the best an address without a UID can do.
func eventsFieldSelector(ref ObjectRef) string {
	if ref.UID != "" {
		return "involvedObject.uid=" + ref.UID
	}
	return "involvedObject.name=" + ref.Name
}
