#!/usr/bin/env bash
# Build script invoked by cli/gh-extension-precompile with the release tag as $1.
# Mirrors the action's default platform matrix but injects the version string.
set -euo pipefail

tag="${1:?release tag required}"
version="${tag#v}"

platforms=(
  darwin-amd64 darwin-arm64
  freebsd-386 freebsd-amd64 freebsd-arm64
  linux-386 linux-amd64 linux-arm linux-arm64
  windows-386 windows-amd64 windows-arm64
)

mkdir -p dist
for p in "${platforms[@]}"; do
  goos="${p%-*}"
  goarch="${p#*-}"
  ext=""
  [ "$goos" = "windows" ] && ext=".exe"
  echo "building $p"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w -X main.version=${version}" -o "dist/${p}${ext}" .
done
