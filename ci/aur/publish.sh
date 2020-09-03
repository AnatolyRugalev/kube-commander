#!/usr/bin/env bash

set -e

ROOT="$(dirname $(dirname $( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )))"

export VERSION=$1
echo "Publishing to AUR as version ${VERSION}"

cd ${ROOT}/ci/aur

export GIT_SSH_COMMAND="ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"

rm -rf .pkg
git clone aur@aur.archlinux.org:kube-commander .pkg 2>&1
cp -f kube-commander .pkg/kube-commander
cp -f kubectl-ui .pkg/kubectl-ui

export SHA256SUM=$(sha256sum ${ROOT}/dist/kubecom-linux_linux_amd64/kubecom | awk '{ print $1 }')

CURRENT_PKGVER=$(cat .pkg/.SRCINFO | grep pkgver | awk '{ print $3 }')
CURRENT_RELEASE=$(cat .pkg/.SRCINFO | grep pkgrel | awk '{ print $3 }')

export PKGVER=${VERSION/-/}

if [[ "${CURRENT_PKGVER}" == "${PKGVER}" ]]; then
    export RELEASE=$((CURRENT_RELEASE+1))
else
    export RELEASE=1
fi

envsubst '$PKGVER $VERSION $RELEASE $SHA256SUM' < .SRCINFO.template > .pkg/.SRCINFO
envsubst '$PKGVER $VERSION $RELEASE $SHA256SUM' < PKGBUILD.template > .pkg/PKGBUILD

cd .pkg
git config user.name "GoReleaser"
git config user.email "goreleaser@goreleaser.com"
git add -A
if [ -z "$(git status --porcelain)" ]; then
  echo "No changes."
else
  git commit -m "Updated to version ${VERSION} release ${RELEASE}"
  git push origin master
fi
