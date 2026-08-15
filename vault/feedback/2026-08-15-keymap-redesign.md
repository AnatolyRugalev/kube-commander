# Keymap redesign: one letter per verb, shift = switchers, ctrl+f = search

- Submitted: 2026-08-15
- Priority: high
- Area: keybindings (whole walk)

After five stories of key-confusion feedback (the n-family in S02, the s-family
in S05), here is the whole intended map. It supersedes the individual key ideas
in the earlier items where they disagree; the design rule is: **a verb owns one
easy key; switching surfaces is a capital letter; search is the browser
convention; destructive actions sit on capital keys.**

| Action | New | Current | Notes |
|---|---|---|---|
| search.cluster | **ctrl+f** | ctrl+s | browser convention; frees ctrl+s |
| ns.switch | **N** | ctrl+n | frees ctrl+n; supersedes the shift-N wording of `2026-08-15-context-switch-key-inconsistent.md` (same letter, N) |
| resources.switch | R | R | unchanged |
| ctx.switch | C | C | unchanged |
| res.delete | **D** | d | destructive → capital |
| res.describe | **d** | D | the read action → lowercase |
| sort.column | s | s | **freed** — no direct sort key; see the S header-focus below |
| sort column picker | **S** | sort.clear | **new interaction**, below; supersedes the popup design in `2026-08-15-sort-column-picker.md` |
| actions.menu | **enter** | a | consistent with `2026-08-15-enter-actions-menu.md`; `a` frees up |

**Sort (S)** — no popup and no `s`: **`S` focuses the table's column-header
row** (the top bar). `h`/`l`/`left`/`right` move across the columns, `enter` on
a column toggles the sort **direction**, and **`esc` brings focus back** to the
rows. `sort.clear` lives inside this mode (a "clear" pick).

**Collisions the fold-in must resolve (displaced bindings need homes):**
- `app.searchPrev` is **N** today — displaced by ns.switch=N. Give it a new key
  or fold it into `n` (next) semantics.
- `nav.pageDown` is **ctrl+f** today — displaced by search.cluster=ctrl+f.
  pageDown's only binding; decide a new key or drop full-page scroll (half-page
  `ctrl+d`/`pgdn` still covers it).

This map answers the s-family reach (`2026-08-15-s-key-family-confusion.md`):
search is ctrl+f, sort is s, column pick is S, nothing shares.
