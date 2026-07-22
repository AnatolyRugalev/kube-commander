package kube

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// tableAcceptHeader asks the API server to render a resource with its
// server-side printer — the same output `kubectl get` shows — instead of the
// raw object list. It requests the meta.k8s.io Table v1 media type and falls
// back to plain JSON so an old server that cannot print a Table still responds
// (the caller then gets an empty Table rather than an error). This is the whole
// point of server-side printing: columns are chosen by the server (and by any
// CRD's `additionalPrinterColumns`), so kubecom shows kubectl-identical columns
// for every resource, including CRDs, without hard-coding any of them.
const tableAcceptHeader = "application/json;as=Table;v=v1;g=meta.k8s.io,application/json"

// Column is one column of a server-printed table, mirroring the fields of
// metav1.TableColumnDefinition the TUI needs: Name is the header text; Type is
// the OpenAPI cell type (string/integer/…); Format flags special columns (the
// primary "name" column carries Format=="name"); Priority orders columns by
// importance (0 = always shown; higher values are hidden first when space is
// tight); Description is the human-readable column doc.
type Column struct {
	Name        string
	Type        string
	Format      string
	Description string
	Priority    int32
}

// ObjectRef is the identity of the object a table row represents, extracted from
// the row's embedded object metadata (server-side Table defaults to
// IncludeObject=Metadata). It is what the action/viewer layers address — delete,
// describe, get-YAML — so a row can be operated on without a second lookup.
// Namespace is empty for cluster-scoped resources.
type ObjectRef struct {
	Namespace string
	Name      string
	UID       string
}

// Row is one row of a server-printed table: Cells holds the printed values in
// column order (strings, numbers, or null, matching the Column types); Object is
// the identity of the underlying object.
type Row struct {
	Cells  []any
	Object ObjectRef
}

// Table is the TUI-facing view of a server-printed resource list: the column
// definitions and the rows, decoupled from client-go's metav1.Table so the TUI
// never imports apimachinery. It is generic over every resource — built-ins and
// CRDs alike — because the columns come from the server, not from kubecom.
type Table struct {
	Columns []Column
	Rows    []Row
}

// decodeTable flattens a JSON-encoded server-side metav1.Table into the
// TUI-facing Table. Each row's embedded object metadata (PartialObjectMetadata,
// the Table default) yields the row's ObjectRef; a row whose object metadata is
// absent or unparsable is kept with a zero ObjectRef rather than dropped
// (degrade, don't crash — principle 3), so a single odd row never blanks a list.
func decodeTable(raw []byte) (*Table, error) {
	t, _, err := decodeTableRV(raw)
	return t, err
}

// decodeTableRV is decodeTable plus the Table's resourceVersion (from the
// embedded ListMeta). The watch layer (M1-05b) needs the resourceVersion to open
// a watch that resumes exactly after the listed state, and to advance it on each
// delta / bookmark so a reconnect resyncs cheaply instead of re-listing.
func decodeTableRV(raw []byte) (*Table, string, error) {
	var mt metav1.Table
	if err := json.Unmarshal(raw, &mt); err != nil {
		return nil, "", fmt.Errorf("kube: decoding table: %w", err)
	}

	t := &Table{
		Columns: make([]Column, 0, len(mt.ColumnDefinitions)),
		Rows:    make([]Row, 0, len(mt.Rows)),
	}
	for _, cd := range mt.ColumnDefinitions {
		t.Columns = append(t.Columns, Column{
			Name:        cd.Name,
			Type:        cd.Type,
			Format:      cd.Format,
			Description: cd.Description,
			Priority:    cd.Priority,
		})
	}
	for _, r := range mt.Rows {
		row := Row{Cells: r.Cells}
		if len(r.Object.Raw) > 0 {
			var pom metav1.PartialObjectMetadata
			if err := json.Unmarshal(r.Object.Raw, &pom); err == nil {
				row.Object = ObjectRef{
					Namespace: pom.Namespace,
					Name:      pom.Name,
					UID:       string(pom.UID),
				}
			}
		}
		t.Rows = append(t.Rows, row)
	}
	return t, mt.ResourceVersion, nil
}

// tableRequest builds the GET that negotiates server-side Table printing for a
// resource. It is shared by the List path (`getTable`, `.Do`) and the Watch path
// (`openTableWatch`, `.Stream` with `watch=true`) so both hit the exact same
// endpoint with the same Accept header — the only difference is the verb tail and
// the opts (Watch/ResourceVersion). namespaced selects whether the request is
// scoped to namespace; opts carries the usual list/watch controls
// (label/field selectors, limit, resourceVersion, watch).
//
// Params are encoded with metav1.ParameterCodec, NOT scheme.ParameterCodec: the
// built-in clientset scheme only knows built-in GroupVersions and cannot convert
// metav1.ListOptions to an arbitrary CRD GroupVersion, so it breaks every
// non-built-in CRD group. metav1.ParameterCodec converts list/watch params for
// any GroupVersion (it is what client-go/dynamic uses) and works for built-ins
// too — do not switch this back.
func tableRequest(
	client rest.Interface,
	gvr schema.GroupVersionResource,
	namespaced bool,
	namespace string,
	opts metav1.ListOptions,
) *rest.Request {
	return client.Get().
		NamespaceIfScoped(namespace, namespaced).
		Resource(gvr.Resource).
		VersionedParams(&opts, metav1.ParameterCodec).
		SetHeader("Accept", tableAcceptHeader)
}

// getTable performs one server-side Table GET against the given REST client and
// decodes the result. It is the injectable core of List: a rest.Interface (real
// or fake) is passed in so the request/decode path is exercised hermetically
// (D18) without a live server.
func getTable(
	ctx context.Context,
	client rest.Interface,
	gvr schema.GroupVersionResource,
	namespaced bool,
	namespace string,
	opts metav1.ListOptions,
) (*Table, error) {
	raw, err := tableRequest(client, gvr, namespaced, namespace, opts).Do(ctx).Raw()
	if err != nil {
		return nil, fmt.Errorf("kube: listing %s: %w", gvr.Resource, err)
	}
	return decodeTable(raw)
}

// restClientForGV builds a REST client scoped to one GroupVersion from the
// cluster's resolved config. It is how List addresses an arbitrary resource
// (built-in or CRD) generically: the dynamic client cannot negotiate the Table
// media type per call, so kubecom talks to the REST layer directly, setting the
// group's API path (/api for the core group, /apis otherwise). Construction is
// local — no server round-trip — so it is safe on the first-paint path.
func (c *Clients) restClientForGV(gv schema.GroupVersion) (rest.Interface, error) {
	cfg := rest.CopyConfig(c.Config)
	cfg.GroupVersion = &gv
	if gv.Group == "" {
		cfg.APIPath = "/api"
	} else {
		cfg.APIPath = "/apis"
	}
	cfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	client, err := rest.RESTClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building REST client for %s: %w", gv.String(), err)
	}
	return client, nil
}

// List fetches a resource as a server-printed Table — kubectl-identical columns
// for any discovered resource, including CRDs. namespace is ignored for
// cluster-scoped resources (r.Namespaced == false); pass "" to list across all
// namespaces for a namespaced resource. opts carries list controls such as
// label/field selectors and a limit.
//
// This is the List half of the live table (M1-05a); the Watch half — a reconnect
// -ing event channel built on this same Table decoding — is M1-05b.
func (c *Clients) List(ctx context.Context, r Resource, namespace string, opts metav1.ListOptions) (*Table, error) {
	client, err := c.restClientForGV(r.GVR.GroupVersion())
	if err != nil {
		return nil, err
	}
	return getTable(ctx, client, r.GVR, r.Namespaced, namespace, opts)
}
