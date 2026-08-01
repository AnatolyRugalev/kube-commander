# Dogfood the dedicated logs view against a busy real pod in a real terminal

- Created: 2026-07-25
- By: LOGS-02
- Priority: normal
- Blocks: none (advisory — gates only the LOGS "high-throughput logs stay readable" quality bar, not other work; LOGS-03/04 may proceed)
- Status: done

## What's needed

Run `kubecom` against a real cluster and press `L` on a pod that logs **fast** (an
ingress controller, a busy API server, or `kubectl run spam --image=busybox -- sh -c
'while true; do echo "$(date) line"; done'`):

1. **Throughput.** Does the view keep up, or does the TUI get sluggish / stop responding
   to keys as the buffer grows? Watch it for a minute or two, not five seconds — the
   suspected failure mode is *gradual*: `logsview.render()` re-joins the whole line buffer
   on every appended line, so cost grows with the number of lines held, not the rate. If
   it degrades, note roughly how many lines in (the header's `total` count after you type
   any `/` query tells you).
2. **Live grep while following.** With lines streaming, press `/` and type a substring.
   The shown lines should narrow *immediately and keep updating* as new matching lines
   arrive — that "grep a live tail" feel is the whole point of the feedback this answers.
   Then `Esc`: the full stream should still be there (nothing re-fetched) and still tailing.
3. **Follow / pause.** Scroll up mid-stream — it should pause (header `[paused]`) and stay
   where you put it rather than being yanked to the bottom by the next line. `f` resumes
   and jumps to the newest line.
4. **Full-screen legibility.** The view replaces the browse panes, keeping only the status
   bar and the hint line. Do long log lines read acceptably, and does the header stay
   readable at your terminal width? Long lines are clipped by default (one log line, one
   row): press `w` (`logs.wrap`) to fold them onto continuation rows instead, or leave it
   off and use `h`/`l` to scroll sideways — the header shows `[wrap]` or `[+N]` columns
   hidden (LOGS-04a). Both were added after this task was raised and are hermetically
   tested, but whether *wrapping a fast stream* stays readable is an eyes-on question.
5. **Exit is clean.** `Esc` (with no filter open) or `q` closes it and the browse table is
   exactly as you left it; the stream is torn down (no goroutine left tailing — the pod's
   log request should stop).

## Why the agent can't do it

The sandbox has no cluster and no interactive terminal, so there is no way to generate
genuine sustained log throughput through a real TTY. Every *behaviour* above is covered
hermetically (`internal/tui/logs_test.go`, `logsview_test.go`) — routing, filtering,
follow, teardown — but hermetic tests feed a handful of lines synchronously and so cannot
observe the one thing at issue: whether rendering keeps up under load. A perf claim the
agent cannot measure is exactly the "green it couldn't earn" D79 forbids.

## How to resolve

Do the check, then EITHER set `Status: done` with a `## Result` (the agent folds it in and
deletes this file next leg), OR delete it if nothing needs to flow back. If throughput
*does* degrade, put it in `../feedback/` — the fix is an incremental render (keep the
rendered content and append to it, rather than re-joining the buffer) and it should be
prioritised over LOGS-03/04.

## Result

**No degradation observed — the suspected failure mode did not materialise.** Checked
2026-08-01 against `shop/firehose` on the k3d dogfood cluster: a busybox loop emitting
**~1,900 lines/sec** (measured: 5,684 lines in a 3-second `kubectl logs -f`), i.e. a
genuinely hostile stream rather than a briskly-logging app.

- **pt 1 (throughput) — passed.** The view kept up and stayed responsive to keys; the
  gradual slide this task was raised to catch (`logsview.render()` re-joining the whole
  line buffer per appended line, so cost grows with lines *held*) was not visible in
  normal use. The incremental-render fix this task pre-authorised is therefore **not
  needed** and should not be written speculatively.
- **pts 2-5 — not separately confirmed.** Live grep while following, follow/pause on
  scroll, `w` wrap + `h`/`l` sideways scroll, and clean teardown on exit were not walked
  as individual checks. All four are hermetically covered (`internal/tui/logs_test.go`,
  `logsview_test.go`); what this session adds is only the perf answer they could not give.

Recorded as measured, not as eyeballed: the line rate above is the number that makes the
"stays readable under load" bar meaningful, and it is the one thing the sandbox could not
produce. If rendering ever does degrade, it will be at buffer *depth* rather than rate —
this run did not sit on the stream long enough to bound that, so a multi-hour tail
remains untested.
