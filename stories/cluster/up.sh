#!/usr/bin/env bash
# stories/cluster/up.sh — (re)create the story cluster from scratch.
#
# Destroys the cluster if it already exists, then builds it again and applies the
# fixture. That is the point: a story compared against a cluster that drifted since the
# last run measures the drift, not the UX (D268 pt 3). Every run starts identical.
#
# Substrate is k3d — k3s in docker — so the cluster is real Kubernetes and still
# disposable. Needs `k3d`, `kubectl` and a running docker.
#
#   ./stories/cluster/up.sh          # rebuild and select the cluster
#   ./stories/cluster/down.sh        # remove it
#
# The `broken` namespace never becomes ready by design, so nothing waits on it.
set -euo pipefail

CLUSTER="${KUBECOM_STORY_CLUSTER:-kubecom-story}"
CONTEXT="k3d-${CLUSTER}"
MANIFESTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/manifests"

for tool in k3d kubectl; do
  command -v "$tool" >/dev/null || { echo "error: $tool is not on PATH" >&2; exit 1; }
done
docker info >/dev/null 2>&1 || { echo "error: docker is not running" >&2; exit 1; }

echo "==> removing any existing '${CLUSTER}' so this run starts clean"
k3d cluster delete "$CLUSTER" >/dev/null 2>&1 || true

echo "==> creating '${CLUSTER}' (1 server, 2 agents)"
# Two agents so a DaemonSet has more than one pod and the scheduler has a real choice
# to make — a single-node fixture hides both.
k3d cluster create "$CLUSTER" --agents 2 --wait

echo "==> applying the fixture"
kubectl --context "$CONTEXT" apply -f "$MANIFESTS"

echo "==> waiting for the healthy workloads (the 'broken' namespace never settles)"
kubectl --context "$CONTEXT" -n shop rollout status deploy/storefront --timeout=180s
kubectl --context "$CONTEXT" -n shop rollout status deploy/checkout --timeout=180s
kubectl --context "$CONTEXT" -n shop rollout status deploy/logspam --timeout=180s
kubectl --context "$CONTEXT" -n data rollout status statefulset/redis --timeout=180s
kubectl --context "$CONTEXT" -n data rollout status daemonset/node-agent --timeout=180s

kubectl config use-context "$CONTEXT" >/dev/null

cat <<EOF

The story cluster is up and selected (context: ${CONTEXT}).

  shop    storefront (ConfigMap), checkout (Secret, 2 containers), logspam, Ingress
  data    redis StatefulSet + bound PVCs, DaemonSet, a completed Job, a CronJob (2m)
  broken  crashloop, bad-image, unschedulable, never-ready, an unbindable PVC

Run a story with a clean kubecom user dir and the keystroke log on:

  ./stories/cluster/run.sh s01

Tear it down with ./stories/cluster/down.sh
EOF
