#!/bin/bash
# Validate that .version file exists and is in correct format

set -e

VERSION_FILE=".version"

if [ ! -f "$VERSION_FILE" ]; then
    echo "❌ Version file not found: $VERSION_FILE"
    exit 1
fi

VERSION=$(cat "$VERSION_FILE")

# Validate semver format (simple check)
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ Invalid version format: $VERSION"
    echo "   Expected: X.Y.Z (e.g., 0.1.8)"
    exit 1
fi

echo "✅ Version file is valid: $VERSION"

# Check if version matches latest git tag (if in git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
    if [ -n "$LATEST_TAG" ]; then
        LATEST_TAG=${LATEST_TAG#v}
        if [ "$VERSION" != "$LATEST_TAG" ]; then
            echo "⚠️  Warning: .version ($VERSION) differs from latest git tag ($LATEST_TAG)"
            echo "   This is OK if you're preparing a new release"
        else
            echo "✅ Version matches latest git tag: v$VERSION"
        fi
    fi
fi
