# Break a CRD's conversion webhook and read what the empty pane says

- Created: 2026-08-02
- By: CRD-01
- Amended: 2026-08-04 by AUTH-04b — item 6 added (the credential-plugin pane). Same surface,
  same question ("does the empty pane say the true thing?"), different failure, and it needs
  an expired SSO session rather than a broken webhook — do whichever you can.
- Amended: 2026-08-05 by AUTH-05b — item 7 added: the same expired session now also opens a
  prompt offering to run the login, and accepting it suspends kubecom into `aws sso login`.
  It shares item 6's setup, so do the two in one sitting.
- Amended: 2026-08-07 by AUTH-07 — item 8 added (nothing paints over the panes any more).
  Same expired session as items 6 and 7 and no extra setup, but it is about what the screen
  does *before* the pane speaks, so read it first if you can.
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

7. **The offer, and the suspend (AUTH-05b, 2026-08-05) — the only place kubecom runs a
   command it composed.** Same expired session as item 6, one step further: once the
   diagnosis lands, a confirm box should open over the browse view titled **Re-authenticate**,
   naming the cause and the exact command (`aws sso login --profile <yours>`), with the pane
   underneath saying "kubecom is asking whether to run this for you" instead of sending you
   to another terminal. `y`/enter accepts, `n`/esc declines.
   - **Decline first.** Nothing should run, the box closes, and the pane should go back to
     "Run this in another terminal, then reopen the resource:" with the command still quoted.
     Reopening the kind re-arms the whole thing (one diagnosis, and so one offer, per
     selection — D214 pt 3), which is how you get a second offer to accept.
   - **Then accept.** kubecom should hand you the terminal — the real one, not the alt
     screen — and `aws sso login` should print its verification code and open your browser
     exactly as it does from a shell. **This is the claim no test can make**: that the
     suspend is clean, that the login is interactive, and that the TUI repaints intact when
     it exits. Say if anything is garbled on the way out or back.
   - **After a successful login**, the rows should arrive without a keypress: the failed
     request is retried (the kind's watch restarts, D215 pt 5). Time it roughly — a second or
     two is expected. If the pane instead sits on the old notice, note that.
   - **After a failed or abandoned login** (ctrl-C out of the browser flow), expect a
     status-bar toast naming the command, its exit status and the first line it printed —
     and *no* second attempt. The full output is in `~/.cache/kubecom/kubecom.log` under
     `remediation failed`; paste it if the toast said nothing useful.
   - **The one thing to watch for.** The command in the box is clipped to about sixty cells,
     so a long profile name may be cut there — the pane behind it carries the same command
     wrapped in full, on purpose. Say whether that reads acceptably or whether the box needs
     to wrap instead.
   - **If the offer never appears** but item 6's diagnosis did: note what was on screen at
     the time. An offer is deliberately suppressed while any picker, viewer, modal, the logs
     view or the filter field is open, and it does not queue for later.

8. **Does the plugin still paint over the panes (AUTH-07, 2026-08-07)?** Items 6 and 7 are
   about what the empty pane *says*; this one is about the frame around it. Until this leg,
   client-go ran the credential plugin with its stderr on kubecom's own terminal, so on every
   refresh — including the successful ones — whatever `aws` printed landed on top of the
   browse view and stayed there until bubbletea happened to repaint those exact lines. fd 2
   now points at `~/.cache/kubecom/kubecom.log` for as long as the UI is up (D247).
   - **On the expired session of item 6**, watch the screen in the seconds before the pane
     changes: no `aws` text, no partial lines through the table's borders, no stray blank
     rows. If something does appear, say roughly *where* it landed and what it said — and
     check whether the same text is also in the log, since "in both places" and "only on
     screen" are different bugs.
   - **On a healthy context that still uses a plugin** (an EKS cluster with a valid session),
     just leave kubecom open past a token refresh — an hour, typically. Nothing should ever
     flicker. This is the case that used to be invisible precisely because it worked.
   - **Then the suspends, which must still show you everything**: `e` into the editor on a
     bad buffer (the editor's own error), an exec shell into a pod whose container is gone,
     and item 7's accepted `aws sso login`. Each of those *should* print to your terminal
     normally. If any of the three has gone silent where it used to speak, that is this leg's
     regression and worth reporting above everything else here.

## Result

_(unanswered)_
