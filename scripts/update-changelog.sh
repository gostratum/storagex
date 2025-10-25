#!/bin/bash
# Update CHANGELOG.md with new version entry

set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "❌ Usage: $0 <version>"
    exit 1
fi

# Remove 'v' prefix if present
VERSION=${VERSION#v}

CHANGELOG_FILE="CHANGELOG.md"
TEMP_FILE="${CHANGELOG_FILE}.tmp"
DATE=$(date +%Y-%m-%d)

echo "📝 Updating CHANGELOG.md for version v$VERSION..."

# Check if changelog exists
if [ ! -f "$CHANGELOG_FILE" ]; then
    echo "❌ CHANGELOG.md not found"
    exit 1
fi

# Check if version already exists in changelog
if grep -q "## \[$VERSION\]" "$CHANGELOG_FILE"; then
    echo "⚠️  Version $VERSION already exists in CHANGELOG.md"
    echo "   Skipping update (you can manually edit if needed)"
    exit 0
fi

# Create new changelog entry
NEW_ENTRY="## [$VERSION] - $DATE

### Added
- Release version $VERSION

### Changed
- Updated gostratum dependencies to latest versions

"

# Insert new entry after [Unreleased] section
if grep -q "## \[Unreleased\]" "$CHANGELOG_FILE"; then
    # Find line number of [Unreleased]
    LINE_NUM=$(grep -n "## \[Unreleased\]" "$CHANGELOG_FILE" | head -1 | cut -d: -f1)
    
    # Insert after unreleased section (skip 2 lines to get past header and blank line)
    {
        head -n "$((LINE_NUM + 1))" "$CHANGELOG_FILE"
        echo ""
        echo "$NEW_ENTRY"
        tail -n "+$((LINE_NUM + 2))" "$CHANGELOG_FILE"
    } > "$TEMP_FILE"
    
    mv "$TEMP_FILE" "$CHANGELOG_FILE"
else
    # No unreleased section, prepend to file
    {
        echo "# Changelog"
        echo ""
        echo "## [Unreleased]"
        echo ""
        echo "$NEW_ENTRY"
        cat "$CHANGELOG_FILE"
    } > "$TEMP_FILE"
    
    mv "$TEMP_FILE" "$CHANGELOG_FILE"
fi

echo "✅ CHANGELOG.md updated with version $VERSION"
echo ""
echo "📋 Please manually update the changelog entry with actual changes:"
echo "   Edit the sections under version $VERSION"
