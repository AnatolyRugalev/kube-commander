# Single enter should reach a search result

- Submitted: 2026-08-15
- Priority: high
- Area: cluster search (walked during S05)

In search results I had to **double-enter** to reach a hit — one enter leaves the
query field (or steps through the result), a second opens it. I want to reach a
result with a **single enter**.

The search view already emits its `SelectedMsg` on one drill-in
(`components/searchview/searchview.go`); the friction is the query-field/result-
list focus hand-off in between. Make one enter from the query both commit and
open the highlighted result (or otherwise collapse the two steps into one).
