# Namespace seam + picker follow-ups (dogfood of FB-ns-menu-seam)

- Submitted: 2026-07-21
- Priority: high
- Area: namespace seam (menu) / namespace picker

Dogfooded the landed namespace seam. Three fixes — the third is a functional
dead-end, hence high priority:

1. **Drop the `"Namespace: "` text prefix; use an arrow indicator instead.** The
   seam row should read like a dropdown — an arrow (e.g. `▾`) plus the value —
   not the literal `Namespace: <x>` label.
   (`internal/tui/components/menu/menu.go:561`,
   `label := clip("Namespace: "+ns, innerW)`.)

2. **Unscoped value should render `(all)`, not `"all namespaces"`.**
   (`internal/tui/components/menu/menu.go:58`, `const namespaceAll = "all
   namespaces"` → `(all)`; update the two menu tests that assert on the old
   string at `menu_test.go:479-480`.) So e.g. the row reads `▾ (all)` when
   unscoped and `▾ kube-system` when scoped.

3. **Can't re-select all-namespaces in the picker (dead-end).** The app launches
   in all-namespaces scope, but the namespace picker only lists concrete
   namespaces — there's no "all namespaces" entry, so once you pick a specific
   namespace you can never get back to the unscoped view without restarting. Add
   an "all namespaces" sentinel entry (maps to empty `ns`, re-scopes the watch to
   all) — ideally pinned at the top of the picker. The picker is seeded at
   `internal/tui/app.go:565` (`m.nsPicker.SetItems(msg.namespaces)`); prepend the
   sentinel there (and map its selection back to `""`).
