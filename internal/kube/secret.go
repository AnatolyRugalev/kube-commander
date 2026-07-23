package kube

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretEntry is one decoded key/value of a Secret's data. Value is the
// base64-decoded content the API stores under the key — the typed clientset
// returns Secret.Data already decoded (map[string][]byte), so this is the raw
// bytes as a string, ready to reveal. Binary values render as-is (the viewer is
// read-only text); a copy slice (M3-08b) can special-case them later.
type SecretEntry struct {
	Key   string
	Value string
}

// SecretData is a Secret's type and its decoded entries, sorted by key so the
// viewer renders them deterministically (the API returns the data map in no
// defined order).
type SecretData struct {
	Type    string
	Entries []SecretEntry
}

// SecretData fetches a Secret and returns its type and base64-decoded data —
// the in-TUI equivalent of `kubectl get secret <name> -o jsonpath` piped through
// `base64 -d`, so a user can inspect secret values without an external shell (D2:
// in-process viewers). It uses the typed clientset (CoreV1().Secrets), which
// decodes the wire base64 into raw bytes for us, so no manual decode is needed and
// a malformed value cannot slip through undecoded.
//
// Empty name is rejected. Errors are wrapped, never panicked: a NotFound (secret
// gone since the row was listed) or an RBAC denial surfaces to the caller to
// display as a transient toast (principle 3).
func (c *Clients) SecretData(ctx context.Context, ref ObjectRef) (SecretData, error) {
	if ref.Name == "" {
		return SecretData{}, fmt.Errorf("kube: secret data: empty object name")
	}
	sec, err := c.Clientset.CoreV1().Secrets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return SecretData{}, fmt.Errorf("kube: getting secret %q: %w", ref.Name, err)
	}
	entries := make([]SecretEntry, 0, len(sec.Data))
	for k, v := range sec.Data {
		entries = append(entries, SecretEntry{Key: k, Value: string(v)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return SecretData{Type: string(sec.Type), Entries: entries}, nil
}
