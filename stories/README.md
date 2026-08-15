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

## Running one

```bash
./stories/cluster/up.sh
kubecom --keylog /tmp/story-01.jsonl
```

Then read the trace back with `kubecom keys analyze` (STORY-03). The most interesting
line in a trace is a keypress that resolved to no action — that is someone reaching for
a key kubecom does not have.

> The stories themselves land in STORY-04, one `.md` per user path. One of them is
> marked the **main story**, and the screencast (TAPE-01) and `docs/usage.md` (DOC-04)
> are both cut against it, so the demo, the docs and the test path stay one product.
