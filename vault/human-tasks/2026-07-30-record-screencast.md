# Record the screencast GIF from `docs/screencast/screencast.tape` and commit it with the README line

- Created: 2026-07-30
- By: M5-09
- Priority: normal
- Blocks: none (advisory — gates only the screencast half of the M5 exit criterion "README +
  keybindings docs current and accurate"; the tape, the `make screencast` target and their
  drift guards have landed, so M5-06…M5-11 all proceed. M5-10's pre-flight should carry this
  item forward, not wait on it.)
- Status: done

## What's needed

M5-09 wrote the tape and the target; what is missing is the recording, because a recording
needs three things the sandbox does not have (see "Why the agent can't do it").

```bash
# 1. install vhs and its recorders (vhs shells out to ttyd + ffmpeg)
go install github.com/charmbracelet/vhs@latest      # or: brew install vhs
#    brew install ttyd ffmpeg   /   apt install ffmpeg + ttyd from its releases

# 2. point kubectl at a cluster worth filming, then:
cd kube-commander && git checkout v1
make screencast                                     # builds ./bin/kubecom, runs the tape
```

`make screencast` writes `docs/screencast.gif`. Then, in the same commit:

1. **Embed it in `README.md`.** Put it right under the intro paragraph, above the
   `🚧 v1 is a ground-up rewrite in progress` block:

   ```markdown
   ![kubecom — browse, filter, logs, describe](docs/screencast.gif)
   ```

   This is not optional book-keeping: `TestScreencastAssetAndReadmeAgree` fails
   `make check` if the GIF exists and the README does not reference it (and equally if
   the README references a GIF that is not there), so the asset and the link land together
   or not at all.
2. **Run `make check`** before pushing — the same test also re-checks every key the tape
   presses against the keymap.
3. Set this file's `Status: done` with a one-line `## Result`, or delete it and drop a note
   in `vault/feedback/` if the tour needs changing.

### What the tape assumes about your cluster

Read the header of [`docs/screencast/screencast.tape`](../../docs/screencast/screencast.tape) — it names two
`TUNE` strings, and they are the only cluster-specific text in it:

- the **table filter** (`kube-system`) — narrows the pod list on screen;
- the **log grep** (`info`) — should match lines in whichever pod is selected after the
  filter, or the grep demo shows an empty screen.

The selected pod is "the second row after filtering", so check that the pod the tour lands
on actually logs something. If the tour needs different keys rather than different strings,
change the tape *and its `# kubecom-action:` annotations together* — the annotations are
what `make check` validates, so a wrong one fails the build rather than misleading a reader.

### What to look at while it records

The GIF is the first thing a visitor sees, so it is also a free UX review. Worth noting if:

- discovery takes visibly longer than the `Sleep 5s` after launch (the tape would be
  filming a spinner — that is either a pacing fix or a real cold-start finding);
- the logs view's throughput looks bad at 24 fps (this overlaps the open
  `2026-07-25-logs-view-throughput-dogfood` task — one recording can answer both);
- anything on screen leaks a **real cluster name, namespace or secret**. The tour never
  opens a Secret, but the status bar names your context throughout, and the describe
  output is whatever the selected pod has. If it leaks, record against a throwaway kind
  cluster instead — the tape does not care which cluster it is.

## Why the agent can't do it

Three separate reasons, each sufficient on its own (D79):

- **`vhs` is not in the sandbox image**, and cannot be fully obtained: the agent *can*
  `go install` vhs itself (it did — the tape is validated with `vhs validate`), but vhs
  records by driving **ttyd** and encoding with **ffmpeg**, neither of which is present and
  neither of which is a Go tool.
- **There is no cluster.** Every frame of this GIF is live cluster data — a pod list, a log
  stream, a describe. There is nothing to film against a fake client.
- **A screencast is a visual judgment call.** Whether the pacing reads well, whether the
  colors survive GIF quantization, and whether the tour tells the right story are questions
  only a human watching the result can answer.

## How to resolve

Record it, commit the GIF plus the README line, then set `Status: done` here with a `## Result`
naming the cluster shape you used and any tape tuning you had to do (so a re-record after a UI
change starts from what worked). If you would rather not host a GIF in the repo at all — it is
a binary that grows every re-record — say so instead, and that is a decision to record: the
alternative is an asset branch or a release-attached file, and the README link changes with it.

## Result

**Done by the maintainer 2026-08-09.** Recorded against the k3d dogfood cluster and
committed as `484e60c` ("docs: Add screencast recording and update vhs setup") —
`docs/screencast.gif` plus the README embed that `TestScreencastAssetAndReadmeAgree`
keeps honest, with tmux-driven captions added in the same pass.

Remaining tuning, from the maintainer, filed as
`../feedback/2026-08-09-screencast-tape-tuning.md`: the cross-cluster search example
finds nothing (bad example), rerunning the tape starts from modified initial state,
and the tour should show more features with more captions.

Next leg: fold in and delete this file; the screencast half of the M5 "README +
keybindings docs current and accurate" criterion can be ticked (a GIF exists, is
linked, and the tape/keymap guards pass), with the tuning feedback tracked separately.
