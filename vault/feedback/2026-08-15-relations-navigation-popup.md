# A relations popup: navigate to a resource's parents, children and linked resources

- Submitted: 2026-08-15
- Priority: high
- Area: navigation / new feature popup (walked during S04)

I want to move from a **pod to its parent workload** (deployment, replicaSet,
statefulSet, daemonSet, job, …) and back, and the same for other resource types
and their reverse relations — and on to anything a resource is *linked* to:
volumes / PVCs, services, etc.

Today only the **owner → pods** direction exists (`res.children` `P`, for the
workload kinds, plus enter-to-drill-in). The reverse — pod → its owner, or
service → its backing pods, or PVC → the volume/pod — has no gesture, and the
S02 walk hit it directly ("is shop affected?" required bouncing between kinds).

I want a **new relations popup**: on any resource, one gesture opens a list of
its related resources (owner, children, selector-matched services/pods, linked
claims/volumes), each row navigable — the reverse of `res.children`, generalized
to every kind and both directions. The fold-in decides the gesture and the
relation graph's exact shape (the owner-address bookkeeping from CTX-MEM-04
already records half of it).
