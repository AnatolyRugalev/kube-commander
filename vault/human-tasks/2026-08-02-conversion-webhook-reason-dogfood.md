# Break a CRD's conversion webhook and read what the empty pane says

- Created: 2026-08-02
- By: CRD-01
- Amended: 2026-08-04 by AUTH-04b — item 6 added (the credential-plugin pane). Same surface,
  same question ("does the empty pane say the true thing?"), different failure, and it needs
  an expired SSO session rather than a broken webhook — do whichever you can.
- Priority: normal
- Blocks: none (advisory — every claim in CRD-01 is covered by hermetic tests, including
  the wording, the wrap and the clear-on-recovery. The single thing a sandbox cannot supply
  is a **real apiserver's** message for a conversion webhook it cannot reach, which is the
  string the named-cause predicate matches on. Nothing on the board waits for this.)
- Status: open

## What's needed

CRD-01 gives the browse table an empty state that says why its LIST failed, and the
headline case is the one the 2026-08-01 dogfood identified (D191 pt 1): a CRD whose
conversion webhook is down makes the **apiserver** fail the list, so `kubectl` fails
identically and there is nothing to fix on this machine. kubecom now says exactly that —
but only if it recognises the failure, and it recognises it by matching the phrase
`conversion webhook for ` in the server's message (`kube.ConversionWebhookFailed`,
`internal/kube/errors.go`). That phrase is apiextensions-apiserver's own wording, taken
from its source, **not** from an observed cluster.

To break one deliberately on the dogfood cluster: install a CRD with
`spec.conversion.strategy: Webhook` (external-secrets with both `v1beta1` and `v1` served
is the original reporter's setup), then scale its webhook deployment to zero, or point the
`clientConfig.service` at a service with no endpoints. `kubectl get externalsecrets -A`
should start failing; that is the state kubecom needs to be opened in.

1. **Does the pane name the webhook, or fall back to the kind's sentence?** Open the kind
   in kubecom. The expected pane is `Cannot list ExternalSecret` over "The API server could
   not call this resource's conversion webhook, so it cannot serve the list at all —
   kubectl fails the same way. This is cluster-side: check the CRD's conversion webhook
   service and its certificate." If instead you get "The API server could not be reached.
   Check your network…", the match missed and the real message is different from the one
   the predicate expects — **paste the `Server:` line verbatim** (it is also in
   `~/.cache/kubecom/kubecom.log` in full, D159). That line is the whole answer.
2. **Is the reason readable at your width?** It word-wraps to the pane, and the `Server:`
   quote can be long (a URL plus a dial error). At a narrow terminal the copy may be cut at
   the bottom of the pane — the headline is first on purpose, but say if the useful part
   fell off.
3. **The sibling kinds.** `SecretStore` and `PushSecret` share the group; the original
   board note asked for them to be checked with it. Same webhook, so expect the same pane
   with their own kind named.
4. **Does it clear when you fix it?** Scale the webhook back up. The watch loop re-Lists on
   a backoff, and the rows should replace the reason **without any keypress** within a few
   seconds (D200 pt 2). If you have to reselect the kind to get rows back, that is a bug.
5. **The 403 case, which needs no broken cluster.** Any kind your user cannot list (or a
   `kubectl auth can-i`-negative one on a restricted context) should show "You are not
   allowed to list it here — RBAC denied the request…". Cheap to check while you are there.

6. **The credential-plugin pane (AUTH-04b, 2026-08-04) — the one thing hermetic tests cannot
   reach.** Different failure, same surface. On a context that authenticates through an exec
   credential plugin (`aws eks get-token` and friends), let the session expire — for AWS SSO,
   `aws sso logout`, or just wait it out — then open any kind in kubecom. What the pane should
   do is: show the kind's generic sentence ("The credential plugin in your kubeconfig failed
   …") for a moment, then **replace it**, within a second or two, with the diagnosed version:
   the profile named, `aws sso login --profile <yours>` as the fix, the failed invocation with
   its exit code, and the AWS CLI's own stderr underneath.
   Three ways this can go wrong, and all three are worth a line back:
   - **The pane never changes.** Then the failure did not classify as the plugin's — kubecom
     matches client-go's own two message shapes (`exec: executable aws failed with exit code
     N`, `… not found`) and nothing else (D195/AUTH-01). Paste the `Server:` line from the
     pane, or the `surfaced error` line from `~/.cache/kubecom/kubecom.log`: that text is the
     whole answer.
   - **It says it has no command to suggest.** Either the stderr wording is one the SSO
     markers miss, or your kubeconfig's stanza names no `--profile`/`AWS_PROFILE` (kubecom
     will not guess one, D212). The log carries a `credential plugin diagnosed` line with the
     captured stderr verbatim — paste that.
   - **It says the re-run worked.** Legitimate if something renewed the credentials in
     between; suspicious if the rows never arrive. Note which.
   Also worth saying: how long the pane sat on the generic sentence before the diagnosis
   landed (the re-run is bounded at 15s, and a slow `aws` is exactly the case that bound is
   for), and whether anything is *missing* off the bottom of the pane at your terminal size —
   the fix is printed above the stderr precisely so that truncation costs evidence rather than
   the answer (D213 pt 2).

## Result

_(unanswered)_
