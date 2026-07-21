# CRDs / non-standard resources need explicit dynamic menu config, per context

- Submitted: 2026-07-21
- Priority: high
- Area: resource menu / configuration

CRDs and other non-standard resource types should be addable to the menu
**explicitly**. This implies a **dynamic, per-context menu configuration**: the set
of resource types shown (and how they're grouped/ordered) is not purely derived
from the built-in seed list but is configurable.

Design direction I want:

- Menu configuration lives in the **kubecom config directory** (the XDG config
  dir we already decided on).
- **One file per context** (I think this is the right granularity — different
  clusters expose different CRDs, so per-context config keeps each cluster's menu
  relevant). Keyed by the kubeconfig context name.
- A context with no config file falls back to the built-in default menu.
- The file should let a user add resource types (e.g. CRDs by group/version/kind),
  and ideally reorder/regroup them.

This is bigger than one leg — triage into concrete board tasks (config schema +
loader, per-context file resolution, menu merge with the seed/discovery set, and
CRD entry format), then land the first slice.
