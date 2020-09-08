#!/bin/sh
set -e

RELEASES_URL="https://github.com/AnatolyRugalev/kube-commander/releases"

TARGET_PATH="./kubecom"

last_version() {
  curl -sL -o /dev/null -w %{url_effective} "$RELEASES_URL/latest" |
    rev |
    cut -f1 -d'/'|
    rev
}

download() {
  test -z "$VERSION" && VERSION="$(last_version)"
  test -z "$VERSION" && {
    echo "Unable to get kubecom version." >&2
    exit 1
  }
  OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
  curl -s -L -o "$TARGET_PATH" \
    "$RELEASES_URL/download/$VERSION/kubecom_${VERSION}_${OS_NAME}_amd64"
}

download
chmod +x $TARGET_PATH
