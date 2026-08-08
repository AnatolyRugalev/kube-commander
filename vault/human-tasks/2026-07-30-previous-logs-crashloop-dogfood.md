# Read a real crash-looping pod's previous-instance logs (`ctrl+p`)

- Created: 2026-07-30
- By: M5-01a
- Amended: 2026-08-08 by LOGS-SEL-03 — item 6 added (the cursor bar under a `/` highlight).
  It needs no crash-looper at all, only a pod with a few hundred log lines, so it is the
  cheapest thing here: do it on whatever pod you open first, before the rest.
- Priority: normal
- Blocks: none (advisory — every claim below is covered by hermetic tests against a fake
  streamer; what a sandbox cannot supply is a real kubelet serving a real terminated
  container's log. The M5 line is unaffected.)
- Status: open

## What's needed

Make a pod crash-loop and read the log of the instance that died. If nothing in the
cluster is obliging:

```bash
kubectl run crashy --image=busybox --restart=Always -- sh -c 'echo starting; sleep 3; echo "fatal: goodbye" >&2; exit 1'
```

Wait for `RESTARTS` to reach 2 or so, then in `kubecom`: select the pod, press `L`, then
press `Ctrl+P`.

1. **Does the previous instance's log actually arrive?** You should see the *completed*
   output of the run that died — `starting` and `fatal: goodbye` — not the partial output
   of the one currently running. The header should read `pod/crashy  [previous]` (and, at
   a multi-container pod, the container name too).
2. **Does the stream end, or does it sit there?** This is the one thing the sandbox could
   not test and the reasoning behind it is inference, not observation. kubecom asks for
   `Follow` and `Previous` together, betting that the kubelet serves the terminated
   instance's log and closes — because the container being read is not running, so the
   follow loop should stop at a clean EOF (D177 pt 2, `internal/kube/logs.go`
   `followLogStream`). If instead the view goes empty, or hangs before delivering the
   lines, or keeps reconnecting every 2 seconds, that bet is wrong and `Previous` needs
   `Follow: false` — say which of those you saw.
3. **Does `Ctrl+P` come back?** A second press should return to the running instance, drop
   `[previous]` from the header, and show the current run's output.
4. **The no-previous-instance case.** Do the same on a pod that has never restarted (any
   healthy one). Expect a status-bar toast carrying the server's own words — something
   like `logs: previous terminated container "…" in pod "…" not found` — and the view to
   close rather than sit empty. Is that message *readable* at your terminal width, or is
   it clipped to uselessness? (It is also in `~/.cache/kubecom/kubecom.log` in full,
   D159 — worth pasting here either way, since the exact wording the apiserver uses is
   not something the sandbox could know.)
5. **Does the grep survive the flip, and should it?** With a query typed (`/fatal`),
   `Ctrl+P` keeps it and re-narrows the other instance's lines (D177 pt 3). The claim is
   that this is what you want — the same question asked of both logs. If it feels wrong
   (e.g. the query from the running instance is noise in the dead one), that is a taste
   call only a user can make, and reverting it is a one-line change (`Restream` →
   `Reset`).
6. **Can you tell the cursor from a match, on the same line?** (Added 2026-08-08 by
   LOGS-SEL-03 — the eye half of a question the rest of which was settled by measurement.)
   Any pod with a few hundred lines will do. Open `L`, type `/` and a query that hits often
   (a word in most lines), then move `j`/`k` onto a matching line and press `v` to start a
   selection. On that one line there are now two backgrounds: the selection bar under the
   whole line, and the yellow match highlight inside it. **Are they two things, or one
   wash?** kubecom deliberately keeps both — blanking the highlight on the cursor's line
   would hide the answer on the line you are reading (D252 pt 1), and the ratio between the
   two backgrounds measures 4.05–9.89:1 across the dark palettes, which is why the
   measurement was taken as the answer. What it cannot tell you is whether it *reads* that
   way at your terminal's contrast and font. If one swallows the other, say which, and on
   which theme — the fix is a role's colors, not the design.
   - **Not on `catppuccin-latte` or `solarized-light`.** Both are known-bad here until
     **THEME-05** lands: the highlight is near-white on yellow (2.15:1 / 2.62:1), which is
     a separate defect that would tell you nothing about this question. Use the default or
     any other dark palette.

## Result

**Partially verified 2026-08-01 — kept open for one question.**

Checked against `broken/crashloop` on the k3d dogfood cluster (a pod with **2,305**
restarts, so no shortage of dead instances to read).

- **pt 1 — passed.** `Ctrl+P` delivers the previous instance's log; the flip works.

Still open, and the reason this file is not closed:

- **pt 2 — UNANSWERED, and it is the one that matters.** Whether the stream *ends* was
  not observed. kubecom requests `Follow` and `Previous` together and D177 pt 2 **bets**
  the kubelet serves the terminated instance and closes at EOF, because the container
  being read is not running (`internal/kube/logs.go`, `followLogStream`). That bet is
  inference, never observation — the sandbox cannot produce a real kubelet. A stream that
  instead hangs, arrives empty, or reconnects every ~2s means the bet is wrong and
  `Previous` needs `Follow: false`.
- **pt 4 — UNANSWERED.** The no-previous-instance toast (on a pod that never restarted)
  was not triggered, so the apiserver's exact wording — and whether it survives a real
  terminal width or clips to uselessness — is still unknown. Worth pasting verbatim from
  `~/.cache/kubecom/kubecom.log` when it is.
- **pts 3, 5 — not exercised.** The return flip, and whether keeping the `/` query across
  `Ctrl+P` (D177 pt 3) is right or noise, were not judged.

To finish: select the crash-looper, `L`, `Ctrl+P`, and **watch it for ~30 seconds** —
does it settle, or keep reconnecting? Then the same on any healthy pod for pt 4. That is
the whole remainder.
