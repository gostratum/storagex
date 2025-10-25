#!/bin/bash
# Semantic version bumping script

set -e

CURRENT_VERSION=$(cat .version 2>/dev/null || echo "0.0.0")
BUMP_TYPE=${1:-patch}

# Remove 'v' prefix if present
CURRENT_VERSION=${CURRENT_VERSION#v}

# Parse version
IFS='.' read -r -a version <<< "$CURRENT_VERSION"
MAJOR="${version[0]}"
MINOR="${version[1]}"
PATCH="${version[2]}"

# Validate current version
if [ -z "$MAJOR" ] || [ -z "$MINOR" ] || [ -z "$PATCH" ]; then
    echo "❌ Invalid current version: $CURRENT_VERSION" >&2
    exit 1
fi

# Bump based on type
case "$BUMP_TYPE" in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
    *)
        echo "❌ Invalid bump type: $BUMP_TYPE" >&2
        echo "Usage: $0 [major|minor|patch]" >&2
        exit 1
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"
echo "$NEW_VERSION" > .version

# Only output the new version on stdout for capture
echo "$NEW_VERSION"
