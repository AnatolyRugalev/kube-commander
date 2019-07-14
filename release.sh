#!/bin/bash

set -e

~/bin/semantic-release -ghr -vf -noci
export VERSION=$(cat .version)
CGO_ENABLED=0 gox -os '!freebsd !netbsd !windows' -arch '!arm' -ldflags="-s -w -X main.version=${VERSION}" -output="bin/{{.Dir}}_v"$VERSION"_{{.OS}}_{{.Arch}}"
#ghr $(cat .ghr) bin/
#24d3d2fc8445a67c17f8b7f09846c3a32518e366