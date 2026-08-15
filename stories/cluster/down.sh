#!/usr/bin/env bash
# stories/cluster/down.sh — remove the story cluster.
#
# Nothing in the fixture is worth keeping: up.sh rebuilds it identically in a couple of
# minutes, which is the whole reason it is committed.
set -euo pipefail

CLUSTER="${KUBECOM_STORY_CLUSTER:-kubecom-story}"

command -v k3d >/dev/null || { echo "error: k3d is not on PATH" >&2; exit 1; }

k3d cluster delete "$CLUSTER"
echo "removed '${CLUSTER}'."
