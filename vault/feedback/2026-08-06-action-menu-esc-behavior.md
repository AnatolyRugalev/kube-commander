# Esc after opening the action menu on a pod should close it, not clear the filter

- Submitted: 2026-08-06
- Priority: normal
- Area: row actions / command palette, key handling

Repro: select a pod, press `a` to open the row-action menu, then press `Esc`.
Expected: it backs out of the action menu, returning me to the pod list where
I was. Actual: `Esc` clears the palette's filter input instead, and the
palette/menu stays open — so `Esc` doesn't actually let me back out, I have
to hit it again (or some other key) to really close it.

Want: the first `Esc` should close the action menu / palette and return focus
to where I was, the same way `Esc` behaves as a "back" action everywhere
else. If a filter is typed *and* the menu should stay open on the first Esc,
that's a product call to make explicitly — but the current behavior (silently
eating the first Esc to clear input while leaving the menu open) is not it.
