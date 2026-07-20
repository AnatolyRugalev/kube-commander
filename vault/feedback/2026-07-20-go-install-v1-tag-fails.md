# README install command `go install …@v1` fails (Go treats `v1` as a version tag)

- Submitted: 2026-07-20
- Priority: normal
- Area: README / install docs

The README's install line is broken:

```
go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@v1
```

**Root cause:** in `go install path@v1`, `@v1` is a module **version query**, and
`v1` matches Go's version-prefix form (major version 1), so the toolchain looks
for the highest `v1.x.x` **semver tag** — the repo has none. A branch merely
*named* `v1` can't be selected this way: when a query is shaped like a version,
semver interpretation wins and Go never falls back to the branch name. Hence it
tries to download a tag and fails.

**Fix (do this):** make **install-from-local-checkout** the primary path — it needs
no tag and respects the branch:

```
git clone -b v1 https://github.com/AnatolyRugalev/kube-commander
cd kube-commander
go install ./cmd/kubecom      # installs to $(go env GOPATH)/bin
```

(or `go build -o kubecom ./cmd/kubecom`). Remove the `@v1` remote form. Optionally
mention a remote one-liner pinned to a commit SHA, which is *not* semver-parsed and
so works: `go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@<commit>`.

A proper `go install …@<version>` / `@latest` path only works once there's a real
release tag — defer that to the **M5 release** milestone (which also renames the
branch to `main`, dissolving the `v1`-vs-semver collision). If useful, add an
M5 note/backlog item that release tagging must restore a clean remote `go install`.

Keep this in sync with the "README stays current" rule (D68).
