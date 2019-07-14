#!/bin/bash

set -e

~/bin/semantic-release -ghr -vf -noci
export VERSION=$(cat .version)
cd cmd/kube-commander
CGO_ENABLED=0 gox -os '!freebsd !netbsd !windows' -arch '!arm' -ldflags="-s -w -X main.version=${VERSION}" -output="../../bin/{{.Dir}}_v"$VERSION"_{{.OS}}_{{.Arch}}"
ghr $(cat .ghr) bin/