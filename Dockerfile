# kubecom container image (M5-08). A **convenience** path, not the recommended
# one: kubecom is a local, zero-deploy TUI you point at your own kubeconfig
# (`vault/goals.md`), so the native binary — release tarball, Homebrew cask, AUR
# package, `go install` — is always the better install. The image exists for
# people who want to try it without putting anything on their PATH, and for CI
# shells that already have a container runtime and nothing else.
#
# Two properties this file exists to hold:
#
#   1. **No build stage.** The 2020 Dockerfile compiled kubecom from source in a
#      `golang:` stage, so the image shipped a *different* binary than the
#      release archives — different toolchain, no version ldflags, unverifiable
#      against the checksums. goreleaser's `dockers_v2` pipe instead lays the
#      already-built release binaries into the build context as
#      `<goos>/<goarch>/<binary>`, so `$TARGETPLATFORM` below copies the exact
#      artifact the tarball, the cask and the AUR package carry.
#   2. **No kubectl.** The 2020 image installed a pinned kubectl (and nano, jq,
#      curl, a nanorc) because the old TUI shelled out for nearly everything.
#      kubecom talks to the apiserver through client-go and shells out to nothing
#      (D2), so the image is the binary and a CA bundle.
#
# Base: distroless `static`, which is a scratch image plus the three things a
# static Go binary that speaks TLS actually needs — ca-certificates,
# /etc/nsswitch.conf and tzdata — and nothing that needs patching. The **root**
# variant is deliberate over `:nonroot`: a kubeconfig is conventionally mode
# 0600, so a bind-mounted one is unreadable to distroless' fixed uid 65532, and
# `--user $(id -u)` in turn leaves $HOME unwritable (Docker resolves an unknown
# uid to `HOME=/`), which stops kubecom before it can open its log file. Root in
# a throwaway local container holding your own kubeconfig is not the threat
# model; an image whose one documented command does not work is a real problem.
FROM gcr.io/distroless/static

# Set by buildx per target platform; goreleaser's build context is laid out to
# match it exactly. Without the ARG the variable is empty and the COPY silently
# resolves to `/kubecom` — TestDockerfileCopiesTheReleasedBinary guards the pair.
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/kubecom /usr/local/bin/kubecom

# The TUI's color depth comes from $TERM. `docker run -t` injects TERM=xterm
# only when the image does not already declare one, and plain `xterm` caps
# kubecom's themes at 16 colors — every theme this repo ships is written for 256
# (`internal/tui/styles`). An explicit -e TERM=... still wins over this.
ENV TERM=xterm-256color

# $HOME is /root here (Docker resolves it from the image's /etc/passwd), which
# is where kubecom looks for ~/.kube/config and where it writes its log
# (~/.cache/kubecom/kubecom.log), config and per-context state. Mount a
# kubeconfig at /root/.kube — see the README's Docker section. Deliberately no
# VOLUME: it would create a stray anonymous volume on every run that forgets
# --rm, and buys nothing a bind mount does not already do.
ENTRYPOINT ["/usr/local/bin/kubecom"]
