# Dogfood the first usable build against a real cluster

- Created: 2026-07-20
- By: REVIEW-05 (progress review)
- Priority: high
- Blocks: M2-10, M2-11, M2-12, M2-13
- Status: open

## What's needed

M2-RUN made `kubecom` launchable and several UI legs have landed since (welcome
page, Dashboard-grouped menu, namespace picker, table filter, in-status-bar error
surfacing) — but **all of it has only ever been verified against fakes/teatest and
a cluster-less PTY, never a real apiserver in a real terminal.** Please run it once
against a live cluster and confirm the "first usable" experience:

```
git clone -b v1 https://github.com/AnatolyRugalev/kube-commander
cd kube-commander
go install ./cmd/kubecom     # or: go build -o kubecom ./cmd/kubecom
kubecom                       # uses your current kubeconfig/context
```

Sanity pass: the grouped menu renders and reads sensibly · drilling into
Pods/Nodes shows live-updating rows · namespace switch (`ctrl+n`) works · filter
(`/`) narrows rows · `?` help overlay shows keys · an induced error (e.g.
`kubecom --context does-not-exist`) degrades gracefully without breaking the
layout · `q` quits cleanly and the terminal is restored (alt-screen).

## Why the agent can't do it

The loop runs in a sandbox with no real cluster and no interactive terminal — it
cannot see how the UI actually looks or behaves. This needs a human's eyes.

## How to resolve

File anything wrong into `../feedback/` (bugs, UX gripes — the loop will pick them
up first). Then set `Status: done` here (or delete this file). While this task is
open it blocks the **new** M2 feature slices (M2-10 modal, M2-11 config
persistence, M2-12 migration, M2-13 sort) so the loop doesn't pile more surface
area onto unvalidated UI — but it deliberately does **not** block feedback fixes,
finishing the in-flight filter (M2-09b), or teatest coverage of existing behavior
(M2-14), so the loop stays productive while it waits.
