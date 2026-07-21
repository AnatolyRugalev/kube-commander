# Namespace picker belongs in the left menu

- Submitted: 2026-07-21
- Priority: normal
- Area: resource menu / namespace picker

Dogfooded the first usable build against a live k3d cluster (`k3d-kubecom-test`,
3 namespaces of mixed workloads). It launches and resources browse fine. Feedback
follows across several files.

The namespace picker should be surfaced **in the left menu**, not only behind
`ctrl+n`. Placing it there also gives us a natural visual seam: it **separates
cluster-wide resources from namespace-scoped ones**. Cluster-scoped types (Node,
PersistentVolume, StorageClass, Namespace, …) sit above/apart from the namespaced
ones, and the picker marks the boundary — so the menu itself communicates the
scope of what you're browsing. `ctrl+n` can remain as the shortcut.
