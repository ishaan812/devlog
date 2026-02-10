#!/bin/bash
set -e

VERSION=${1}

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 0.0.1"
  exit 1
fi

echo "Publishing @ishaan812/devlog v${VERSION} to npm..."

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

# Publish to npm (--access public for scoped package)
echo ""
echo "Publishing to npm..."
npm publish --access public

# Publish to GitHub Packages (uses GITHUB_TOKEN or NODE_AUTH_TOKEN for npm.pkg.github.com)
echo ""
echo "Publishing to GitHub Packages..."
if [ -n "${GITHUB_TOKEN}" ] || [ -n "${NODE_AUTH_TOKEN}" ]; then
  TMP_NPMRC=$(mktemp)
  echo "//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN:-$NODE_AUTH_TOKEN}" >> "$TMP_NPMRC"
  echo "@ishaan812:registry=https://npm.pkg.github.com" >> "$TMP_NPMRC"
  NPM_CONFIG_USERCONFIG="$TMP_NPMRC" npm publish --registry=https://npm.pkg.github.com
  rm -f "$TMP_NPMRC"
else
  echo "  ⚠ Skipped (set GITHUB_TOKEN for GitHub Packages publish)"
fi

# Create git tag
git add package.json
git commit -m "Release v${VERSION}"
git tag v${VERSION}
git push origin master
git push origin v${VERSION}

echo ""
echo "✓ Successfully published @ishaan812/devlog v${VERSION}!"
echo ""
echo "Users can now install with:"
echo "  npm install -g @ishaan812/devlog"
echo "  # or"
echo "  go install github.com/ishaan812/devlog/cmd/devlog@latest"
