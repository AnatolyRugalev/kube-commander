# Dogfood the Edit → $EDITOR flow against a real cluster in a real terminal

- Created: 2026-07-24
- By: M3-15b
- Priority: normal
- Blocks: none (advisory — gates only the M3 "Edit round-trips through $EDITOR" exit criterion, not other work)
- Status: open

## What's needed

Run `kubecom` against a real cluster in a real terminal and exercise the Edit
action (default key `e`, or "Edit" in the actions menu) on an editable object
(e.g. a Deployment or ConfigMap):

1. Press `e` on a selected row → your `$EDITOR` (or `$KUBE_EDITOR`) opens with the
   object's YAML.
2. **No-change path:** exit the editor without changing anything (or `:q` in vim) →
   the status bar should show `edit: no changes · <Kind ns/name>` and the object is
   untouched.
3. **Apply path:** make a valid change (e.g. bump `spec.replicas`, add a label), save
   and quit → the status bar should show `applied · <Kind ns/name>` and the change is
   live on the cluster (`kubectl get -o yaml` confirms).
4. **Reject paths degrade, never clobber:** an invalid-YAML save, a renamed
   `metadata.name`, or a concurrently-changed object should surface an error toast and
   leave the object unmodified (kube.Update guards these, M3-15a/D129).
5. **TUI restores cleanly** after the editor exits (no garbled screen), same as exec.

Try both `EDITOR=vi` (the default) and, if you like, a GUI editor with a wait flag
(`EDITOR="code -w"`) to confirm the argv split works.

## Why the agent can't do it

The sandbox has no real cluster and no interactive terminal, so the actual `$EDITOR`
suspend (a full-screen editor owning the TTY, then the TUI re-capturing it) cannot be
driven here — teatest ≠ a human's eyes (same reason exec needed a dogfood, D125). The
routing, temp-file round-trip, no-change detection, and apply are covered hermetically;
only the live interactive suspend + real apply need a human.

## How to resolve

Do the check, then EITHER set `Status: done` with a `## Result` (the agent ticks the M3
Edit exit criterion and deletes this file next leg), OR delete it if nothing needs to
flow back. Any bug goes in `../feedback/`.
