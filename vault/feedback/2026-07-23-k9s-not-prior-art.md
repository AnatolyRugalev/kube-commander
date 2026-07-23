# Don't call k9s "prior art" — it's a contemporary of kube-commander

- Submitted: 2026-07-23
- Priority: normal
- Area: README wording / project framing

`README.md` (Special thanks) currently lists:

> - [k9s](https://github.com/derailed/k9s) — prior art in the Kubernetes-TUI space

That framing is wrong. **kube-commander and k9s were born at roughly the same
time** (both emerged around 2019–2020), so k9s is a **contemporary / peer**
project, not "prior art" that kube-commander came after or built upon. "Prior art"
implies a predecessor relationship that doesn't exist.

**Fix:** reword that line to frame k9s as a contemporary / kindred Kubernetes TUI
in the same space — e.g. "a contemporary Kubernetes TUI in the same space" or "a
kindred terminal Kubernetes UI (contemporary with kube-commander)". Just drop the
"prior art" characterization.

Leave the other k9s references as-is — they're accurate: "simpler and more
discoverable than k9s" (comparison), the "not cloning k9s" non-goal, "tview (what
k9s uses)", and the "k9s-`:`-style" palette references are all fine.
