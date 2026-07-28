# Judge fuzzy cluster-search match quality against a real cluster

- Created: 2026-07-28
- By: SEARCH-04c-2b
- Priority: normal
- Blocks: none (advisory — the SEARCH line is closed and every claim below is covered by
  hermetic tests; this is a *taste* question the sandbox cannot answer)
- Status: open

## What's needed

Run `kubecom` against a real cluster with a decent number of objects, press `Ctrl+s`, and
type queries the way you actually would. The mechanism is tested; what is not tested is
whether the results *feel* right.

1. **Does the fallback earn its keep?** Type an abbreviation you'd naturally use —
   `apisrv` for `api-server`, `kdns` for `kube-dns`, `certmgr` for `cert-manager`. The
   object you meant should be in the list. If it is not, the matcher is too strict.
2. **Is the noise below the signal, and is that enough?** Fuzzy hits are ranked strictly
   below every name that contains the query outright. Scroll past the exact matches: is the
   tail useful, or is it junk you have to scroll through to be sure you saw everything? If
   the latter, the answer is probably a *floor* (drop scattered hits below some score)
   rather than a re-rank — say so and it becomes a board item.
3. **Short queries.** Two or three characters subsequence-match a great deal. Type `db`,
   `ns`, `ca`. Does the list stay usable, or does the tail swamp the head? A minimum needle
   length for the fuzzy fallback was deliberately *not* added (no arbitrary constant without
   evidence) — this is the evidence.
4. **The budget's share.** At most a quarter of the 200-hit cap may be fuzzy
   (`searchScatteredShare` in `internal/kube/search.go`). Widen the scope with `Ctrl+a`
   and/or `Ctrl+w` on a loose query: do you ever notice results missing, and does the tail
   feel too long or too short? `limit/4` is a guess, and a reversible one.
5. **Regression check.** A query that names something exactly (`nginx`, a full pod name)
   must return what it always did, in the same order, with the fuzzy results strictly after.
   If a fuzzy hit ever appears *above* an exact one, that is a bug in the band gap and worth
   reporting immediately — it is the invariant the whole design rests on (D152 pt 3/D153).

## Result

_(fill in — then the next agent leg folds this in and deletes the file)_
