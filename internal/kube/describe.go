package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/describe"
)

// describeChunkSize bounds each page of the Events list the describer fetches to
// build the "Events:" trailer, matching kubectl's default describe chunking. It
// keeps a describe of a busy object from pulling the whole event stream in one
// request; the describer pages until it has the events it needs.
const describeChunkSize = 500

// Describe renders the human-readable `kubectl describe` output for the object a
// table row references, in-process (D2 — no kubectl binary, no external pager).
// It reuses kubectl's own describe generators (k8s.io/kubectl/pkg/describe) so the
// output is byte-identical to `kubectl describe`, including the per-kind sections
// (a Pod's containers/conditions/volumes, a Deployment's rollout status, …) and
// the trailing "Events:" table.
//
// It is generic over every resource. Built-in kinds get their specialized
// describer (PodDescriber, DeploymentDescriber, …) keyed by GroupKind; anything
// without one — every CRD, and the rarer built-ins — falls back to the generic
// unstructured describer, which prints name/namespace/labels/annotations plus a
// recursive dump of the object body and the same events trailer. The namespace is
// taken from the ref and ignored for cluster-scoped resources (a cluster-scoped
// describer Gets with an empty namespace).
//
// Unlike GetYAML this takes no context: kubectl's describe package exposes no
// context-aware entry point (it Gets the object and searches events with
// context.TODO internally), so there is nothing to thread one through. The TUI
// runs Describe off the render goroutine and abandons the result if the view
// closes. Empty name is rejected. Errors are wrapped, never panicked: a NotFound
// (object gone since the row was listed) or an RBAC denial surfaces to the caller
// to display (#86).
func (c *Clients) Describe(r Resource, ref ObjectRef) (string, error) {
	if ref.Name == "" {
		return "", fmt.Errorf("kube: describe %s: empty object name", r.GVR.Resource)
	}
	describer, err := describerFor(r, c.Config)
	if err != nil {
		return "", fmt.Errorf("kube: describing %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	out, err := describer.Describe(ref.Namespace, ref.Name, describe.DescriberSettings{
		ShowEvents: true,
		ChunkSize:  describeChunkSize,
	})
	if err != nil {
		return "", fmt.Errorf("kube: describing %s %q: %w", r.GVR.Resource, ref.Name, err)
	}
	return out, nil
}

// describerFor picks the describer for a resource, mirroring the selection
// kubectl's describe.NewDescriber does: prefer the specialized built-in describer
// keyed by GroupKind, and fall back to the generic unstructured describer (which
// works for any GVR, so CRDs are covered) otherwise. Client construction here is
// local — the describers build their clients from cfg but make no server call, so
// this stays hermetic and the network round-trip only happens on Describe.
//
// The generic describer is fed a RESTMapping built directly from the discovery
// Resource (restMappingFor) rather than resolved through the RESTMapper: the
// Resource already carries the GVR, GVK, and scope the generic describer reads, so
// there is no reason to make discovery re-derive them.
func describerFor(r Resource, cfg *rest.Config) (describe.ResourceDescriber, error) {
	if d, ok := describe.DescriberFor(r.GVK.GroupKind(), cfg); ok {
		return d, nil
	}
	if d, ok := describe.GenericDescriberFor(restMappingFor(r), cfg); ok {
		return d, nil
	}
	return nil, fmt.Errorf("no describer available for %s", r.GVK)
}

// restMappingFor builds the meta.RESTMapping the generic describer needs from a
// discovery Resource. Pure and side-effect free, so describerFor stays testable
// without a client. Only the fields the generic describer reads are populated —
// Resource (the GVR it Gets through the dynamic client), GroupVersionKind, and
// Scope (namespaced vs cluster, which decides whether the ref's namespace is
// honored).
func restMappingFor(r Resource) *meta.RESTMapping {
	scope := meta.RESTScopeRoot
	if r.Namespaced {
		scope = meta.RESTScopeNamespace
	}
	return &meta.RESTMapping{
		Resource:         r.GVR,
		GroupVersionKind: r.GVK,
		Scope:            scope,
	}
}
