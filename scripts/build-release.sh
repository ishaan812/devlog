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

# DuckDB requires CGO, so cross-compilation needs native C toolchains.
# On macOS: darwin arm64/amd64 work natively; for Linux/Windows install:
#   brew install filosottile/musl-cross/musl-cross mingw-w64
# Or use GitHub Actions for full multi-platform builds (see .github/workflows/release.yml).

echo "Building macOS binaries (CGO_ENABLED=1)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o ${OUTPUT_DIR}/devlog-darwin-amd64 ./cmd/devlog
echo "  ✓ darwin-amd64"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o ${OUTPUT_DIR}/devlog-darwin-arm64 ./cmd/devlog
echo "  ✓ darwin-arm64"

# Try Linux/Windows builds (requires cross-compilers: brew install filosottile/musl-cross/musl-cross mingw-w64)
echo ""
echo "Attempting Linux/Windows builds (may fail without cross-compilers)..."

if CC=x86_64-linux-musl-gcc CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags "-extldflags -static" -trimpath -o ${OUTPUT_DIR}/devlog-linux-amd64 ./cmd/devlog 2>/dev/null; then
  echo "  ✓ linux-amd64"
else
  echo "  ⚠ linux-amd64 skipped (no cross-compiler)"
fi

if CC=aarch64-linux-musl-gcc CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags "-extldflags -static" -trimpath -o ${OUTPUT_DIR}/devlog-linux-arm64 ./cmd/devlog 2>/dev/null; then
  echo "  ✓ linux-arm64"
else
  echo "  ⚠ linux-arm64 skipped (no cross-compiler)"
fi

if CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -trimpath -o ${OUTPUT_DIR}/devlog-windows-amd64.exe ./cmd/devlog 2>/dev/null; then
  echo "  ✓ windows-amd64"
else
  echo "  ⚠ windows-amd64 skipped (no cross-compiler)"
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
