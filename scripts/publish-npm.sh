#!/bin/bash
set -e

VERSION=${1}

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 1.0.0"
  exit 1
fi

echo "Publishing devlog v${VERSION} to npm..."

# Update version in package.json
npm version ${VERSION} --no-git-tag-version

# Build Go binaries
./scripts/build-release.sh ${VERSION}

# Create GitHub release with binaries
echo ""
echo "Creating GitHub release..."
gh release create v${VERSION} \
  --title "v${VERSION}" \
  --generate-notes \
  release/*

# Publish to npm
echo ""
echo "Publishing to npm..."
npm publish

# Create git tag
git add package.json
git commit -m "Release v${VERSION}"
git tag v${VERSION}
git push origin master
git push origin v${VERSION}

echo ""
echo "✓ Successfully published devlog v${VERSION}!"
echo ""
echo "Users can now install with:"
echo "  npm install -g devlog"
echo "  # or"
echo "  go install github.com/ishaan812/devlog/cmd/devlog@latest"
