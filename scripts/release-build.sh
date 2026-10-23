#!/bin/bash
set -euo pipefail
if [[ $# != 2 ]]; then echo "usage: $0 <tag> <outdir>" >&2; exit 1; fi
tag=$1
version=${tag#v}
[[ $tag == v* && $version =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][a-zA-Z0-9.-]+)?$ ]] || { echo 'invalid release tag' >&2; exit 1; }
root=${TSP_RELEASE_SOURCE_DIR:-$(cd "$(dirname "$0")/.." && pwd)}
cd "$root"
declared=$(awk '$1 == "var" && $2 == "Version" {gsub(/"/, "", $4); print $4}' internal/buildinfo/buildinfo.go)
[[ $version == "$declared" ]] || { echo "tag $tag differs from buildinfo.Version $declared" >&2; exit 1; }
commit=$(git rev-parse HEAD)
mkdir -p "$2"
out=$(cd "$2" && pwd)
for arch in amd64 arm64; do
    stage=$(mktemp -d "$out/.stage-$arch.XXXXXX")
    # Keep all staging inside the requested output directory; cleanup on every exit.
    trap 'rm -rf "$stage"' EXIT
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
        -ldflags "-X github.com/alesierraalta/tsp/internal/buildinfo.Version=$version -X github.com/alesierraalta/tsp/internal/buildinfo.Commit=$commit" \
        -o "$stage/tsp" ./cmd/tsp
    cp LICENSE NOTICE README.md "$stage/"
    tar -czf "$out/tsp_${version}_linux_${arch}.tar.gz" -C "$stage" tsp LICENSE NOTICE README.md
    rm -rf "$stage"
    trap - EXIT
done
(cd "$out" && sha256sum "tsp_${version}_linux_amd64.tar.gz" "tsp_${version}_linux_arm64.tar.gz" > checksums.txt)
