#!/usr/bin/env bash
# Publishes split Go modules from the monorepo to the pub repo.
#
# Packages and their dependencies are auto-detected from the source code
# using `go list`. No manual config needed beyond vanity domain and pub repo.
#
# Usage: ./.github/scripts/publish-split-modules.sh <version>
#   e.g.: ./.github/scripts/publish-split-modules.sh 1.2.0
#
# Requires: go, jq, git
# Environment: GH_TOKEN (for pushing to the pub repo)

set -euo pipefail

VERSION="${1:?Usage: $0 <version>}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="$REPO_ROOT/split-modules.json"

GO_VERSION=$(grep '^go ' "$REPO_ROOT/go.mod" | awk '{print $2}')
MONOREPO_MODULE=$(grep '^module ' "$REPO_ROOT/go.mod" | awk '{print $2}')
VANITY_DOMAIN=$(jq -r '.vanityDomain' "$CONFIG")
PUB_REPO=$(jq -r '.pubRepo' "$CONFIG")

WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

echo "Publishing split modules v${VERSION}"
echo "  Go version: ${GO_VERSION}"
echo "  Monorepo module: ${MONOREPO_MODULE}"
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

# Auto-discover packages: directories with non-test .go files, relative to repo root.
# Only includes packages that have at least one non-test .go source file.
echo "Discovering packages..."
PACKAGES_FILE="$WORK_DIR/packages.txt"
cd "$REPO_ROOT"
go list -f '{{.ImportPath}} {{.GoFiles}}' ./... | while IFS= read -r line; do
  imp="${line%% *}"
  files="${line#* }"
  # Skip packages with no non-test source files (GoFiles excludes test files)
  if [ "$files" = "[]" ]; then
    continue
  fi
  # Strip module prefix to get relative dir
  dir="${imp#${MONOREPO_MODULE}/}"
  # Skip root package (if any)
  if [ "$dir" != "$imp" ]; then
    echo "$dir"
  fi
done > "$PACKAGES_FILE"

echo "  Found packages: $(paste -sd', ' "$PACKAGES_FILE")"

# Auto-detect imports per package using go list
IMPORTS_FILE="$WORK_DIR/imports.txt"
go list -f '{{.ImportPath}} {{join .Imports ","}}' ./... | while IFS=' ' read -r imp imports; do
  dir="${imp#${MONOREPO_MODULE}/}"
  if [ "$dir" != "$imp" ]; then
    echo "${dir}|${imports}"
  fi
done > "$IMPORTS_FILE"

# Clear pub repo contents (full replace each release)
find "$WORK_DIR/pub" -mindepth 1 -maxdepth 1 -not -name '.git' -exec rm -rf {} +

# Process each package
while IFS= read -r dir; do
  echo "Processing ${dir}..."

  # Create directory
  mkdir -p "$WORK_DIR/pub/$dir"

  # Copy non-test .go files (maxdepth 1 to avoid pulling subpackage files)
  find "$REPO_ROOT/$dir" -maxdepth 1 -name '*.go' -not -name '*_test.go' -exec cp {} "$WORK_DIR/pub/$dir/" \;

  # Copy README if present
  [ -f "$REPO_ROOT/$dir/README.md" ] && cp "$REPO_ROOT/$dir/README.md" "$WORK_DIR/pub/$dir/"

  # Parse imports for this package
  imports=$(grep "^${dir}|" "$IMPORTS_FILE" | cut -d'|' -f2)

  # Classify imports into kit deps and external deps
  kit_deps=""
  ext_deps=""
  if [ -n "$imports" ]; then
    IFS=',' read -ra imp_arr <<< "$imports"
    for imp in "${imp_arr[@]}"; do
      if [[ "$imp" == "${MONOREPO_MODULE}/"* ]]; then
        # Kit dependency — extract the relative dir
        kd="${imp#${MONOREPO_MODULE}/}"
        if [ -z "$kit_deps" ]; then
          kit_deps="$kd"
        else
          kit_deps="$kit_deps"$'\n'"$kd"
        fi
      elif [[ "$imp" != *"."* ]] || [[ "$imp" == "C" ]]; then
        # stdlib (no dots in path) or cgo — skip
        continue
      else
        # External dependency — find the module root from go.mod
        # Match against known dependency module paths
        matched=""
        while IFS=' ' read -r mod_path mod_ver; do
          if [[ "$imp" == "$mod_path" ]] || [[ "$imp" == "$mod_path/"* ]]; then
            # Only add if not already present
            if [ -z "$ext_deps" ]; then
              ext_deps="$mod_path"
              matched="1"
            elif ! echo "$ext_deps" | grep -qx "$mod_path"; then
              ext_deps="$ext_deps"$'\n'"$mod_path"
              matched="1"
            else
              matched="1"
            fi
            break
          fi
        done < "$DEP_VERSIONS_FILE"
        if [ -z "$matched" ]; then
          echo "  WARNING: No go.mod entry for import: $imp (may be stdlib or transitive)" >&2
        fi
      fi
    done
  fi

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
          echo "	${VANITY_DOMAIN}/${kd} v${VERSION}"
        done <<< "$kit_deps"
      fi

      echo ")"
    fi
  } > "$WORK_DIR/pub/$dir/go.mod"
done < "$PACKAGES_FILE"

# Rewrite import paths: monorepo module -> vanity domain
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
while IFS= read -r dir; do
  tag="${dir}/v${VERSION}"
  echo "Tagging ${tag}"
  git tag -f "$tag"
done < "$PACKAGES_FILE"

# Push
echo "Pushing to ${PUB_REPO}..."
git push origin main --tags --force

echo "Done! Published ${VERSION} to ${PUB_REPO}"
