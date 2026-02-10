#!/bin/bash
set -e

VERSION=${1:-"dev"}
OUTPUT_DIR="release"

echo "Building devlog v${VERSION}..."
echo ""

# Create output directory
mkdir -p ${OUTPUT_DIR}

# Detect current platform
CURRENT_OS=$(go env GOOS)
CURRENT_ARCH=$(go env GOARCH)
echo "Current platform: ${CURRENT_OS}/${CURRENT_ARCH}"
echo ""

# DuckDB requires CGO. Cross-compiling to Linux from macOS doesn't work (DuckDB's
# prebuilt libs need glibc; musl has symbol mismatches). Use Docker for Linux locally.
# Windows: brew install mingw-w64. Full multi-platform: push a tag for GitHub Actions.

echo "Building macOS binaries (CGO_ENABLED=1)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o ${OUTPUT_DIR}/devlog-darwin-amd64 ./cmd/devlog
echo "  ✓ darwin-amd64"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o ${OUTPUT_DIR}/devlog-darwin-arm64 ./cmd/devlog
echo "  ✓ darwin-arm64"

# Windows: cross-compile with mingw-w64 (brew install mingw-w64)
echo ""
echo "Building Windows binary..."
if CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -trimpath -o ${OUTPUT_DIR}/devlog-windows-amd64.exe ./cmd/devlog 2>/dev/null; then
  echo "  ✓ windows-amd64"
else
  echo "  ⚠ windows-amd64 skipped (brew install mingw-w64)"
fi

# Linux: use Docker (DuckDB needs glibc; cross-compile from macOS doesn't work)
echo ""
echo "Building Linux binaries (via Docker)..."
if command -v docker &>/dev/null; then
  if docker run --rm --platform linux/amd64 \
    -v "$(pwd):/app" -w /app \
    -e CGO_ENABLED=1 -e GOOS=linux -e GOARCH=amd64 \
    golang:1.24 go build -trimpath -o ${OUTPUT_DIR}/devlog-linux-amd64 ./cmd/devlog 2>/dev/null; then
    echo "  ✓ linux-amd64"
  else
    echo "  ⚠ linux-amd64 failed"
  fi
  if docker run --rm --platform linux/arm64 \
    -v "$(pwd):/app" -w /app \
    -e CGO_ENABLED=1 -e GOOS=linux -e GOARCH=arm64 \
    golang:1.24 go build -trimpath -o ${OUTPUT_DIR}/devlog-linux-arm64 ./cmd/devlog 2>/dev/null; then
    echo "  ✓ linux-arm64"
  else
    echo "  ⚠ linux-arm64 failed"
  fi
else
  echo "  ⚠ linux-amd64 skipped (install Docker, or push tag for GitHub Actions)"
  echo "  ⚠ linux-arm64 skipped (install Docker, or push tag for GitHub Actions)"
fi

echo ""
echo "✓ Built binaries:"
ls -lh ${OUTPUT_DIR}/

echo ""
echo "To create a GitHub release with these binaries:"
echo "  gh release create v${VERSION} --title 'v${VERSION}' --notes 'Release notes here' ${OUTPUT_DIR}/*"
echo ""
echo "For full multi-platform builds, push a tag to trigger GitHub Actions:"
echo "  git tag v${VERSION} && git push origin v${VERSION}"
