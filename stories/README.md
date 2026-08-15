# Stories

A **story** is a goal a user has, walked against a known cluster, with the keystrokes
recorded — the instrument this project uses to find out whether kubecom is actually
pleasant to use (D268).

A story states the state it begins from, the task in the user's words, and what to
observe. It deliberately **does not say which keys to press**: which keys you reach for
is the measurement. If the story tells you to press `L`, the trace only proves you can
follow instructions.

## The cluster

Every story runs against the fixture in [`cluster/`](cluster/), which is committed so
the ground does not move between runs:

```bash
./stories/cluster/up.sh      # destroys and rebuilds — always the same starting state
./stories/cluster/down.sh    # remove it
```

It is [k3d](https://k3d.io) — k3s in docker — so it is real Kubernetes and still
disposable. Needs `k3d`, `kubectl` and a running docker; a rebuild takes a couple of
minutes.

| namespace | what's in it | why |
|---|---|---|
| `shop` | storefront (nginx + ConfigMap), checkout (Secret, two containers), logspam, Services, Ingress | the healthy state a story starts from; the subjects for logs, the secret viewer and the container picker |
| `data` | redis StatefulSet + bound PVCs, DaemonSet, a completed Job, a CronJob every 2 min | the kinds a Deployment-only fixture never reaches; the CronJob also makes the table change under a live watch, unprompted |
| `broken` | crashloop, bad-image, unschedulable, never-ready, an unbindable PVC | five failures with five *different* answers — logs, events, the scheduler, the probe, the claim |

The `broken` namespace never becomes ready. That is the point, and `up.sh` does not wait
on it.

## The stories

| id | path | what it walks |
|---|---|---|
| S01 | [`s01-first-contact.md`](s01-first-contact.md) | get oriented on a cluster you have never seen |
| S02 | [`s02-the-failing-pod.md`](s02-the-failing-pod.md) | the **main story** — five failures, five different answers |
| S03 | [`s03-read-the-logs.md`](s03-read-the-logs.md) | find the failures in a chatty stream |
| S04 | [`s04-change-something.md`](s04-change-something.md) | scale, restart, and edit by hand |
| S05 | [`s05-half-a-name.md`](s05-half-a-name.md) | find an object when you only half-remember its name |

S02 is the **main story** — the screencast (TAPE-01) and `docs/usage.md` (DOC-04) are
both cut against it, so the demo, the docs and the test path stay one product.

## Running one

```bash
./stories/cluster/up.sh
./stories/cluster/run.sh s02               # trace -> ~/traces/s02.jsonl
```

`run.sh` launches kubecom with a **throwaway user dir** (`XDG_CONFIG_HOME`/
`XDG_CACHE_HOME` under `$TMPDIR`, wiped before every launch), so a walk starts with
no config, no per-context menus/state, no discovery cache and no log history — the
same guarantee `up.sh` gives the cluster — and never reads or writes your real
`~/.config/kubecom`. The trace still lands in `~/traces/<id>.jsonl`, outside the
wipe, so it survives.

Then read the trace back with `kubecom keys analyze ~/traces/s02.jsonl`: it
reports the unresolved presses ranked by frequency, the action counts, the longest
pauses (where the walker stopped to think), and the sequences that were started
but never finished. The most interesting line in a raw trace is a keypress that
resolved to no action — that is someone reaching for a key kubecom does not have.
