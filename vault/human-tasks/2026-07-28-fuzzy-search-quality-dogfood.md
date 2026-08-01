# Judge fuzzy cluster-search match quality against a real cluster

- Created: 2026-07-28
- By: SEARCH-04c-2b
- Priority: normal
- Blocks: none (advisory — the SEARCH line is closed and every claim below is covered by
  hermetic tests; this is a *taste* question the sandbox cannot answer)
- Status: done

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

Checked 2026-08-01 against a k3d cluster (`k3d-kubecom-test`, ~30 pods across 5
namespaces plus the traefik/gateway CRD set). Verdict: **the fuzzy fallback works and
earns its keep.** No change requested.

What was actually exercised, so a later leg does not over-read this:

- **pt 1 (does the fallback earn its keep) — yes.** `strfrnt` scoped to `shop` returned
  6 results, every one a real `storefront` object (Service, Deployment, ConfigMap
  `storefront-config`, and the three `storefront-57c9cf5fb7-*` pods) and nothing else.
  That is the abbreviation case the point asks about, answered cleanly.
- **pt 2 (is the noise below the signal) — no noise to rank.** At this scope every hit
  was a true positive, so the question of whether the fuzzy tail is junk you must scroll
  past did not arise. A score *floor* is therefore still neither justified nor refuted.
- **pts 3, 4, 5 — not exercised.** Short queries (`db`/`ns`/`ca`), the widened scope
  (`Ctrl+a`/`Ctrl+w`) and `searchScatteredShare = limit/4`, and a deliberate check of the
  exact-above-fuzzy band-gap invariant were all left unrun. The invariant was not
  *violated* in what was seen, but it was not probed either.

So: closed as "no problem found", not as "every point verified". `limit/4` and the
absence of a minimum needle length both remain guesses that a busier cluster could still
overturn — worth re-opening only if the tail ever feels wrong in real use.
