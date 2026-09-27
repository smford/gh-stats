#!/usr/bin/env bash
set -euo pipefail

# bump-version.sh: Calculates the next Semantic Version based on git commit history.
# Conforms to Conventional Commits:
#   - Breaking changes ("BREAKING CHANGE:" or "feat!:", "fix!:") -> Bumps MAJOR
#   - New features ("feat:" or "feat(...)") -> Bumps MINOR
#   - Bug fixes, docs, refactors, chores, or default merges -> Bumps PATCH

# Fetch tags if in git repo
git fetch --tags --force 2>/dev/null || true

# Find the latest semver tag
LATEST_TAG=$(git tag -l "v*.*.*" --sort=-v:refname | head -n 1 || true)

if [ -z "$LATEST_TAG" ]; then
  # Initial release
  NEW_TAG="v0.1.0"
  BUMP_TYPE="initial"
  echo "No existing semver tag found. Initializing at $NEW_TAG" >&2
else
  # Strip leading 'v'
  VERSION_NUM="${LATEST_TAG#v}"
  IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION_NUM"

  # Defined code paths that warrant a release
  CODE_PATHS=("cmd" "pkg" "go.mod" "go.sum" "action.yml" "scripts")

  # Check if there are any actual code modifications since the latest tag
  CODE_DIFF=$(git diff --name-only "${LATEST_TAG}..HEAD" -- "${CODE_PATHS[@]}" 2>/dev/null || true)

  if [ -z "$CODE_DIFF" ]; then
    echo "No code changes detected in cmd/, pkg/, go.mod/sum, or action.yml since $LATEST_TAG. Skipping release." >&2
    NEW_TAG="$LATEST_TAG"
    BUMP_TYPE="none"
  else
    # Examine only commits modifying code files since the latest tag
    COMMITS=$(git log "${LATEST_TAG}..HEAD" --oneline -- "${CODE_PATHS[@]}" 2>/dev/null || true)

    BUMP_TYPE="patch"

    # Check for breaking changes
    if echo "$COMMITS" | grep -Eq 'BREAKING CHANGE:|^[a-z]+(\([a-z0-9_-]+\))?!:'; then
      BUMP_TYPE="major"
    # Check for features
    elif echo "$COMMITS" | grep -Eq '^([0-9a-f]+ )?feat(\([a-z0-9_-]+\))?:'; then
      BUMP_TYPE="minor"
    fi

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
    esac

    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
  fi
fi

# Output results
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "version=${NEW_TAG}"
    echo "version_number=${NEW_TAG#v}"
    echo "bump_type=${BUMP_TYPE}"
    if [ "$NEW_TAG" != "$LATEST_TAG" ]; then
      echo "is_new_version=true"
      # Extract major version (e.g. "v0" or "v1")
      MAJOR_TAG=$(echo "$NEW_TAG" | grep -oE '^v[0-9]+')
      echo "major_tag=${MAJOR_TAG}"
    else
      echo "is_new_version=false"
    fi
  } >> "$GITHUB_OUTPUT"
fi

echo "$NEW_TAG"
