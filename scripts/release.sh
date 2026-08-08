#!/usr/bin/env bash
# Builds aienv for a matrix of platforms, packages each as a tar.gz with a
# SHA256SUMS.txt, tags the repo, and publishes a GitHub release via `gh`.
#
# Usage:
#   scripts/release.sh vX.Y.Z
#
# Env overrides:
#   PLATFORMS="linux/amd64 linux/arm64"   space-separated GOOS/GOARCH pairs
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must look like vX.Y.Z (got: $VERSION)" >&2
  exit 1
fi

for tool in go git gh sha256sum tar; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "error: required tool '$tool' not found in PATH" >&2
    exit 1
  fi
done

if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh is not authenticated (run: gh auth login)" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is not clean, commit or stash first" >&2
  exit 1
fi

if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "error: tag $VERSION already exists" >&2
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...

PLATFORMS="${PLATFORMS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64}"

DIST="$REPO_ROOT/dist"
rm -rf "$DIST"
mkdir -p "$DIST"

for platform in $PLATFORMS; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  pkg_dir="aienv-${VERSION}-${GOOS}-${GOARCH}"

  echo "==> building ${GOOS}/${GOARCH}"
  mkdir -p "$DIST/$pkg_dir"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$DIST/$pkg_dir/aienv" \
    ./cmd/aienv
  cp README.md AGENTS.md "$DIST/$pkg_dir/"

  ( cd "$DIST" && tar -czf "${pkg_dir}.tar.gz" "$pkg_dir" && rm -rf "$pkg_dir" )
done

echo "==> checksums"
( cd "$DIST" && sha256sum -- *.tar.gz > SHA256SUMS.txt )

echo "==> tagging $VERSION"
git tag -a "$VERSION" -m "aienv $VERSION"
git push origin "$VERSION"

echo "==> creating GitHub release"
gh release create "$VERSION" \
  "$DIST"/*.tar.gz \
  "$DIST/SHA256SUMS.txt" \
  --title "$VERSION" \
  --generate-notes

url="$(gh release view "$VERSION" --json url -q .url)"
echo "==> done: $url"
