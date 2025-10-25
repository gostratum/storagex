#!/bin/bash
# Auto-update all gostratum dependencies to latest versions

set -e

echo "🔍 Fetching latest gostratum module versions..."




# Only update direct gostratum dependencies (not indirect)
# Parse go.mod require block only (POSIX compatible)
DEPS=()
awk '/^require \(/ {in_req=1; next} /^\)/ {in_req=0} in_req && /github.com\/gostratum\// {print $1}' go.mod | sort -u | while read -r dep; do
    DEPS+=("$dep")
done

for dep in "${DEPS[@]}"; do
    echo "📦 Updating $dep..."
    # Get latest version from proxy
    LATEST=$(go list -m -versions "$dep" 2>/dev/null | awk '{print $NF}')
    if [ -z "$LATEST" ]; then
        echo "   ⚠️  Could not find latest version, trying @latest tag..."
        go get "$dep@latest" 2>/dev/null || echo "   ⚠️  Update failed, keeping current version"
        continue
    fi
    echo "   → Found latest: $LATEST"
    go get "$dep@$LATEST"
done

echo "🧹 Tidying go.mod..."
go mod tidy

echo "✅ Dependencies updated successfully!"
echo ""
echo "📋 Updated dependencies:"
go list -m github.com/gostratum/core github.com/gostratum/metricsx 2>/dev/null || true
