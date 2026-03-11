#!/usr/bin/env bash
# Publishes split Go modules from the monorepo to the pub repo.
#
# Usage: ./scripts/publish-split-modules.sh <version>
#   e.g.: ./scripts/publish-split-modules.sh 1.2.0
#
# Requires: jq, git
# Environment: GH_TOKEN (for pushing to the pub repo)

set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$SCRIPT_DIR/split-modules.json"

GO_VERSION=$(grep '^go ' "$REPO_ROOT/go.mod" | awk '{print $2}')
VANITY_DOMAIN=$(jq -r '.vanityDomain' "$CONFIG")
PUB_REPO=$(jq -r '.pubRepo' "$CONFIG")
MONOREPO_MODULE="github.com/jrschumacher/wails-kit"

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

echo "Publishing split modules v${VERSION}"
echo "  Go version: ${GO_VERSION}"
echo "  Vanity domain: ${VANITY_DOMAIN}"
echo "  Pub repo: ${PUB_REPO}"

# Clone or init pub repo
if [ -n "${GH_TOKEN:-}" ]; then
  git clone "https://x-access-token:${GH_TOKEN}@github.com/${PUB_REPO}.git" "$WORK_DIR/pub" 2>/dev/null || {
    echo "Pub repo not found, initializing..."
    mkdir -p "$WORK_DIR/pub"
    cd "$WORK_DIR/pub"
    git init
    git remote add origin "https://x-access-token:${GH_TOKEN}@github.com/${PUB_REPO}.git"
    cd "$REPO_ROOT"
  }
else
  echo "No GH_TOKEN set, using SSH"
  git clone "git@github.com:${PUB_REPO}.git" "$WORK_DIR/pub" 2>/dev/null || {
    echo "Pub repo not found, initializing..."
    mkdir -p "$WORK_DIR/pub"
    cd "$WORK_DIR/pub"
    git init
    git remote add origin "git@github.com:${PUB_REPO}.git"
    cd "$REPO_ROOT"
  }
fi

# Extract dependency versions from root go.mod into a temp file for lookup
DEP_VERSIONS_FILE="$WORK_DIR/dep_versions.txt"
sed -n '/^require/,/^)/p' "$REPO_ROOT/go.mod" | \
  grep -E '^\t' | \
  sed 's/^\t//;s/ \/\/ indirect$//' | \
  awk '{print $1 " " $2}' > "$DEP_VERSIONS_FILE"

# Lookup function: get version for a dependency module
dep_version() {
  local dep="$1"
  awk -v d="$dep" '$1 == d {print $2}' "$DEP_VERSIONS_FILE"
}

# Clear pub repo contents (full replace each release)
find "$WORK_DIR/pub" -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +

# Process each package
for pkg in $(jq -r '.packages | keys[]' "$CONFIG"); do
  dir=$(jq -r ".packages[\"$pkg\"].dir" "$CONFIG")

  echo "Processing ${pkg} (dir: ${dir})..."

  # Create directory
  mkdir -p "$WORK_DIR/pub/$dir"

  # Copy non-test .go files (maxdepth 1 to avoid pulling subpackage files)
  find "$REPO_ROOT/$dir" -maxdepth 1 -name '*.go' ! -name '*_test.go' -exec cp {} "$WORK_DIR/pub/$dir/" \;

  # Copy README if present
  [ -f "$REPO_ROOT/$dir/README.md" ] && cp "$REPO_ROOT/$dir/README.md" "$WORK_DIR/pub/$dir/"

  # Collect deps as newline-separated strings (avoids bash array issues with set -u)
  ext_deps=$(jq -r ".packages[\"$pkg\"].externalDeps[]?" "$CONFIG")
  kit_deps=$(jq -r ".packages[\"$pkg\"].kitDeps[]?" "$CONFIG")

  # Generate go.mod
  {
    echo "module ${VANITY_DOMAIN}/${dir}"
    echo ""
    echo "go ${GO_VERSION}"

    if [ -n "$ext_deps" ] || [ -n "$kit_deps" ]; then
      echo ""
      echo "require ("

      if [ -n "$ext_deps" ]; then
        while IFS= read -r dep; do
          ver=$(dep_version "$dep")
          if [ -z "$ver" ]; then
            echo "  ERROR: No version found for external dep: $dep" >&2
            exit 1
          fi
          echo "	${dep} ${ver}"
        done <<< "$ext_deps"
      fi

      if [ -n "$kit_deps" ]; then
        while IFS= read -r kd; do
          kd_dir=$(jq -r ".packages[\"$kd\"].dir" "$CONFIG")
          echo "	${VANITY_DOMAIN}/${kd_dir} v${VERSION}"
        done <<< "$kit_deps"
      fi

      echo ")"
    fi
  } > "$WORK_DIR/pub/$dir/go.mod"
done

# Rewrite import paths: github.com/jrschumacher/wails-kit/ -> abnl.dev/wails-kit/
echo "Rewriting import paths..."
find "$WORK_DIR/pub" -name '*.go' -print0 | while IFS= read -r -d '' file; do
  sed "s|\"${MONOREPO_MODULE}/|\"${VANITY_DOMAIN}/|g" "$file" > "$file.tmp" && mv "$file.tmp" "$file"
done

# Add a root README
cat > "$WORK_DIR/pub/README.md" << 'HEREDOC'
# wails-kit-pub

Auto-generated split Go modules for [wails-kit](https://github.com/jrschumacher/wails-kit).

**Do not edit this repo directly.** All development happens in the monorepo.

Each subdirectory is an independent Go module importable via vanity URL:

```go
import "abnl.dev/wails-kit/appdirs"
import "abnl.dev/wails-kit/settings"
import "abnl.dev/wails-kit/database"
```
HEREDOC

# Commit
cd "$WORK_DIR/pub"
git add -A

if git diff --cached --quiet; then
  echo "No changes to commit"
else
  git commit -m "Release v${VERSION}

Source: https://github.com/jrschumacher/wails-kit/releases/tag/v${VERSION}"
fi

# Tag each package
for pkg in $(jq -r '.packages | keys[]' "$CONFIG"); do
  dir=$(jq -r ".packages[\"$pkg\"].dir" "$CONFIG")
  tag="${dir}/v${VERSION}"
  echo "Tagging ${tag}"
  git tag -f "$tag"
done

# Push
echo "Pushing to ${PUB_REPO}..."
git push origin main --tags --force

echo "Done! Published ${VERSION} to ${PUB_REPO}"
