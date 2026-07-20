package kube

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Namespaces lists the cluster's namespace names, sorted alphabetically. It backs
// the TUI namespace picker (M2-08c): namespaces are a core, always-present
// resource, so the typed CoreV1 client is used directly rather than the generic
// Table path. A connection fault or RBAC denial returns a wrapped error the caller
// degrades on (Classify — principle 3), never a panic; an empty cluster returns an
// empty slice and no error.
func (c *Clients) Namespaces(ctx context.Context) ([]string, error) {
	list, err := c.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing namespaces: %w", err)
	}
	out := make([]string, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, list.Items[i].Name)
	}
	sort.Strings(out)
	return out, nil
}
