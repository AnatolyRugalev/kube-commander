#!/usr/bin/env bash
# stories/cluster/run.sh — launch kubecom against the story cluster with a clean
# user dir, so every story run starts identical (D268 pt 3).
#
# up.sh makes the *cluster* the same on every run; this makes kubecom the same.
# Each launch wipes a throwaway XDG dir, so the walk starts with no config, no
# per-context menus/state (pins, previous container, the welcome screen), no
# discovery cache and no log history — and it never reads or writes the
# maintainer's real ~/.config/kubecom or ~/.cache/kubecom (the tape's trick, D261).
#
#   ./stories/cluster/up.sh
#   ./stories/cluster/run.sh s02                    # trace -> ~/traces/s02.jsonl
#   ./stories/cluster/run.sh s02 --keylog /tmp/x    # trace elsewhere
#
# The trace defaults to ~/traces/<id>.jsonl — outside the throwaway dir — so a
# story's trace survives the wipe and `kubecom keys analyze` can read it back.
set -euo pipefail

CLUSTER="${KUBECOM_STORY_CLUSTER:-kubecom-story}"
CONTEXT="k3d-${CLUSTER}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

id="${1:-}"
case "$id" in
  s0[1-5]) shift ;;
  *) echo "error: first argument must be a story id (s01..s05)" >&2
     echo "usage: $0 <s01|s02|s03|s04|s05> [extra kubecom args...]" >&2
     exit 1 ;;
esac

# Binary: the repo's built bin/kubecom if present, else kubecom on PATH.
if [ -x "$REPO_ROOT/bin/kubecom" ]; then
  KUBECOM_BIN="$REPO_ROOT/bin/kubecom"
else
  KUBECOM_BIN="$(command -v kubecom || true)"
fi
if [ -z "$KUBECOM_BIN" ]; then
  echo "error: no kubecom binary — build one: go build -o bin/kubecom ./cmd/kubecom" >&2
  exit 1
fi

# Throwaway user dir, wiped before every launch so a rerun starts identical.
XDG="${TMPDIR:-/tmp}/kubecom-stories/$id"
rm -rf "$XDG"
mkdir -p "$XDG" "$HOME/traces"
XDG="$(cd "$XDG" && pwd)"

# The default trace path applies unless the caller passed their own --keylog.
trace="$HOME/traces/$id.jsonl"
if [ -n "$*" ] && [[ " $* " == *" --keylog "* ]]; then
  trace=
fi

if command -v kubectl >/dev/null 2>&1; then
  cur="$(kubectl config current-context 2>/dev/null || true)"
  [ "$cur" = "$CONTEXT" ] || \
    echo "note: current kube-context is '${cur:-?}'; the stories expect '$CONTEXT' (run up.sh)" >&2
fi

echo "==> $id: $KUBECOM_BIN --keylog ${trace:-<yours>}"
if [ -n "$trace" ]; then
  XDG_CONFIG_HOME="$XDG/config" XDG_CACHE_HOME="$XDG/cache" \
    "$KUBECOM_BIN" "$@" --keylog "$trace"
else
  XDG_CONFIG_HOME="$XDG/config" XDG_CACHE_HOME="$XDG/cache" \
    "$KUBECOM_BIN" "$@"
fi

if [ -n "$trace" ] && [ -s "$trace" ]; then
  echo
  echo "==> trace at $trace — read it back with:"
  echo "    $KUBECOM_BIN keys analyze $trace"
fi
