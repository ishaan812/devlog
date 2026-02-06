#!/bin/bash
set -e

VERSION=${1:-"dev"}
OUTPUT_DIR="release"

echo "Building devlog v${VERSION} for all platforms..."

# Create output directory
mkdir -p ${OUTPUT_DIR}

# Build for each platform
GOOS=darwin GOARCH=amd64 go build -o ${OUTPUT_DIR}/devlog-darwin-amd64 ./cmd/devlog
GOOS=darwin GOARCH=arm64 go build -o ${OUTPUT_DIR}/devlog-darwin-arm64 ./cmd/devlog
GOOS=linux GOARCH=amd64 go build -o ${OUTPUT_DIR}/devlog-linux-amd64 ./cmd/devlog
GOOS=linux GOARCH=arm64 go build -o ${OUTPUT_DIR}/devlog-linux-arm64 ./cmd/devlog
GOOS=windows GOARCH=amd64 go build -o ${OUTPUT_DIR}/devlog-windows-amd64.exe ./cmd/devlog

echo "✓ Built binaries:"
ls -lh ${OUTPUT_DIR}/

echo ""
echo "To create a GitHub release with these binaries:"
echo "  gh release create v${VERSION} --title 'v${VERSION}' --notes 'Release notes here' ${OUTPUT_DIR}/*"
