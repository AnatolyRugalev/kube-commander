# Paste the actual error kubecom logs when the `ExternalSecret` CRD fails to open

- Created: 2026-07-29
- By: DIAG-01
- Priority: high
- Blocks: CRD-01
- Status: open

## What's needed

Your feedback (`2026-07-29-external-secrets-crd-error`) said opening the
external-secrets `ExternalSecret` custom resource errors out, and that the actual error
needed finding. It could not be found from the report because kubecom had nowhere to put
one — the error was a 5-second, width-clipped status-bar toast and then gone. As of this
leg it is also written to the log file, so one reproduction now yields the cause.

1. Pull `v1` and build (`go build ./cmd/kubecom`), or reinstall.
2. In a second terminal: `tail -f ~/.cache/kubecom/kubecom.log` (on macOS,
   `~/Library/Caches/kubecom/kubecom.log`).
3. Run `kubecom` against the cluster with the external-secrets CRDs and open
   `ExternalSecret` the way you did before.
4. Copy the `level=ERROR msg="surfaced error"` line (or `msg="discovery failed"` /
   `msg="discovery group unavailable"`) — the whole line, including `kind=` and `error=`.

While you are there, two cheap extras that would narrow it a lot:

- Does `kubectl get externalsecrets -A` work at the same moment, from the same context?
  If it fails too, the fault is cluster-side (an external-secrets **conversion webhook**
  is the usual suspect) and CRD-01 becomes "degrade more legibly", not "fix the client".
- Do the sibling kinds in the same group behave the same — `SecretStore`,
  `ClusterSecretStore`, `PushSecret`? "All of `external-secrets.io` fails" and "only
  `ExternalSecret` fails" point at different causes.

## Why the agent can't do it

The sandbox has no cluster at all, let alone one running the external-secrets operator,
and the CRD is not something that can be faked usefully: the plausible causes — a
conversion webhook the apiserver reports on LIST, a Table-conversion 406, an RBAC 403
scoped to that group, a decode edge case in a printer column — are indistinguishable
without the message and call for opposite fixes, one of which is "kubecom is behaving
correctly and should just say so more clearly". Picking between them by guessing would be
inventing a bug and shipping a fix for it (D79).

## How to resolve

Paste the log line(s) under `## Result` and set `Status: done` — the next leg turns CRD-01
into a normal, unblocked leg and deletes this file. If the reproduction turns out to be
cluster-side and there is nothing for kubecom to do, say so and delete the file; CRD-01
gets closed with that note.
