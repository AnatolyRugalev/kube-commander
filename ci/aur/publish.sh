#!/usr/bin/env bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd ${DIR}

export GIT_SSH_COMMAND="ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"

rm -rf .pkg
git clone aur@aur.archlinux.org:kube-commander .pkg

export TAG=$(cat .tag)
NAME=kube-commander_${TAG}_Linux_x86_64.tar.gz
ARCHIVE=$(dirname $(dirname ${DIR}))/dist/${NAME}
export SHA256SUM=$(sha256sum ${ARCHIVE} | awk '{ print $1 }')

CURRENT_TAG=$(cat .pkg/.SRCINFO | grep pkgver | awk '{ print $3 }')
CURRENT_TAG_RELEASE=$(cat .pkg/.SRCINFO | grep pkgrel | awk '{ print $3 }')

if [[ "${CURRENT_TAG}" == "${TAG}" ]]; then
    export TAG_RELEASE=$((CURRENT_TAG_RELEASE+1))
else
    export TAG_RELEASE=1
fi

envsubst '$TAG $TAG_RELEASE $SHA256SUM' < .SRCINFO.template > .pkg/.SRCINFO
envsubst '$TAG $TAG_RELEASE $SHA256SUM' < PKGBUILD.template > .pkg/PKGBUILD

cd ${DIR}/.pkg
git add .SRCINFO PKGBUILD
git commit -m "Updated to version ${TAG} release ${TAG_RELEASE}"
git push origin master
