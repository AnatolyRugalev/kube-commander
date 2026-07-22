# CRDs from non-built-in groups fail to list/watch (wrong ParameterCodec)

- Submitted: 2026-07-22
- Priority: high
- Area: kube layer — list/watch (`internal/kube/table.go`)

Listing/watching CRDs whose group isn't in the built-in clientset scheme fails
with an empty view and errors like:

```
kube: listing backendtlspolicies: v1.ListOptions is not suitable for converting to "gateway.networking.k8s.io/v1" in scheme "pkg/runtime/scheme.go:100"
kube: listing traefikservices: v1.ListOptions is not suitable for converting to "traefik.io/v1alpha1" in scheme "pkg/runtime/scheme.go:100"
```

**Root cause:** `tableRequest` (`internal/kube/table.go:133`) encodes options with
`scheme.ParameterCodec`, where `scheme` is `k8s.io/client-go/kubernetes/scheme` —
the built-in clientset scheme. It only knows built-in GroupVersions, so
`VersionedParams(&opts, scheme.ParameterCodec)` can't convert `metav1.ListOptions`
to an arbitrary CRD GroupVersion (`gateway.networking.k8s.io/v1`,
`traefik.io/v1alpha1`, …). Built-in kinds work; **every non-built-in CRD group
breaks** — which defeats the "generic over any resource incl. CRDs" goal (#76/#87).

**Fix:** encode with **`metav1.ParameterCodec`** (`k8s.io/apimachinery/pkg/apis/meta/v1`)
instead of `scheme.ParameterCodec`. It converts `metav1.ListOptions`/watch params
for **any** GroupVersion (it's what `client-go/dynamic` uses) and still works for
built-ins — a strict improvement. One line in `tableRequest`; the watch path reuses
`tableRequest` so it's fixed too.

**Test:** add a regression that drives `tableRequest`/`VersionedParams` (or List)
against a **non-built-in** GVR (e.g. a fake `example.com/v1`) and asserts it no
longer errors on parameter conversion — the current hermetic tests only use
built-in GVs, which is why this slipped past `make check`.
