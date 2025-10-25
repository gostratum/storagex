#!/bin/bash
# Complete automated release process for dbx module

set -e

BUMP_TYPE=${1:-patch}
DRY_RUN=${DRY_RUN:-false}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$MODULE_DIR"

echo "🚀 Starting release process (type: $BUMP_TYPE)..."
echo "   Module: dbx"
echo "   Dry run: $DRY_RUN"
echo ""

# 1. Validate version file
echo "✅ Step 1/8: Validating version file..."
./scripts/validate-version.sh

# 2. Ensure clean working directory (if in git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    echo "🔍 Step 2/8: Checking git status..."
    if [[ -n $(git status --porcelain) ]]; then
        echo "⚠️  Warning: Working directory has uncommitted changes"
        echo "   Files will be committed as part of release"
    else
        echo "   ✅ Working directory is clean"
    fi
else
    echo "⚠️  Step 2/8: Not in a git repository, skipping git checks"
fi

# 3. Update dependencies
echo "📦 Step 3/8: Updating gostratum dependencies..."
./scripts/update-deps.sh

# 4. Run tests
echo "🧪 Step 4/8: Running tests..."
if ! make test > /dev/null 2>&1; then
    echo "❌ Tests failed! Aborting release."
    echo "   Run 'make test' to see details"
    exit 1
fi
echo "   ✅ All tests passed"

# 5. Get current version
CURRENT_VERSION=$(cat .version)
echo "📊 Step 5/8: Current version: v$CURRENT_VERSION"

# 6. Bump version
echo "🔢 Step 6/8: Bumping version ($BUMP_TYPE)..."
NEW_VERSION=$(./scripts/bump-version.sh "$BUMP_TYPE")
echo "   📈 New version: v$NEW_VERSION"

# 7. Update changelog
echo "📝 Step 7/8: Updating CHANGELOG..."
./scripts/update-changelog.sh "$NEW_VERSION"

# 8. Commit and tag (if not dry run and in git repo)
if [ "$DRY_RUN" = "false" ]; then
    if git rev-parse --git-dir > /dev/null 2>&1; then
        echo "💾 Step 8/8: Committing and tagging..."
        
        # Add changed files
        git add .version go.mod go.sum CHANGELOG.md
        
        # Commit
        git commit -m "chore(release): bump version to v$NEW_VERSION

- Update gostratum dependencies to latest
- Update CHANGELOG.md
- Bump version from v$CURRENT_VERSION to v$NEW_VERSION

This release was automated via scripts/release.sh"
        
        # Create tag
        git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION

See CHANGELOG.md for details."
        
        echo ""
        echo "✅ Release v$NEW_VERSION completed successfully!"
        echo ""
        echo "📋 Next steps:"
        echo "   1. Review the changes:"
        echo "      git show HEAD"
        echo ""
        echo "   2. Push to remote:"
        echo "      git push origin main"
        echo "      git push origin v$NEW_VERSION"
        echo ""
        echo "   3. Create GitHub release (if applicable):"
        echo "      https://github.com/gostratum/dbx/releases/new?tag=v$NEW_VERSION"
    else
        echo "⚠️  Step 8/8: Not in git repository, skipping commit/tag"
        echo ""
        echo "✅ Version bumped to v$NEW_VERSION"
        echo "📋 Files updated:"
        echo "   - .version"
        echo "   - go.mod / go.sum"
        echo "   - CHANGELOG.md"
    fi
else
    echo ""
    echo "🏷️  DRY RUN: Would create version v$NEW_VERSION"
    if git rev-parse --git-dir > /dev/null 2>&1; then
        echo "💾 DRY RUN: Would commit and tag:"
        echo "   - Commit message: 'chore(release): bump version to v$NEW_VERSION'"
        echo "   - Tag: v$NEW_VERSION"
        echo "⬆️  DRY RUN: Would push to remote"
    fi
    echo ""
    echo "✅ Dry run completed!"
    echo "   Run without DRY_RUN=true to actually release:"
    echo "   make release TYPE=$BUMP_TYPE"
    echo ""
    echo "⚠️  Rolling back changes..."
    git checkout .version go.mod go.sum CHANGELOG.md 2>/dev/null || true
fi
