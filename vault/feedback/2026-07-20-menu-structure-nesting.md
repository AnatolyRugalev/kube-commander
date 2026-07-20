# Left menu items look disorganized — need structure (cluster / namespace nesting)

- Submitted: 2026-07-20
- Priority: normal
- Area: resource menu (left sidebar)

The left menu is currently a flat list of resource kinds and reads as
disorganized. It should be **grouped/nested** by scope, the way the original
kube-commander and the classic Kubernetes Dashboard organize it — roughly:

- **Cluster-level** (not namespaced): Namespaces, Nodes, PersistentVolumes,
  StorageClasses, ClusterRoles, etc.
- **Namespace-level** (scoped to the current namespace), ideally sub-grouped like
  the dashboard does: Workloads (Deployments, StatefulSets, DaemonSets, Pods,
  Jobs, CronJobs…), Config (ConfigMaps, Secrets), Network (Services, Ingresses),
  Storage (PVCs), Access Control (Roles, ServiceAccounts)…

Look at the `master` branch (original kube-commander) for the exact grouping it
used as a reference — we want a similar, familiar structure. Keep it consistent
with the discovery reconcile (grouping must survive CRDs/extra groups being
appended) and with vim navigation (collapse/expand or just section headers —
your call, record the decision).
