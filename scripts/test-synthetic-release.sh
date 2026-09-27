#!/usr/bin/env bash
set -euo pipefail

# test-synthetic-release.sh
# End-to-end synthetic test for gh-stats Release Comparison Mode (--target=release / --target=range).

echo "🧪 Starting Synthetic Release Comparison Test..."

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "${TEMP_DIR}"' EXIT

BINARY_PATH="${TEMP_DIR}/gh-stats"
REPO_DIR="${TEMP_DIR}/synthetic-repo"
SARIF_OUT="${TEMP_DIR}/release.sarif"
JSON_OUT="${TEMP_DIR}/metrics.json"

# Build gh-stats binary
echo "🔨 Building gh-stats binary..."
go build -o "${BINARY_PATH}" ./cmd/gh-stats

# Initialize synthetic git repository
echo "📁 Initializing synthetic repository at ${REPO_DIR}..."
mkdir -p "${REPO_DIR}"
git -C "${REPO_DIR}" init -b main
git -C "${REPO_DIR}" config user.name "Synthetic Release Tester"
git -C "${REPO_DIR}" config user.email "tester@example.com"
git -C "${REPO_DIR}" config commit.gpgsign false

# Base Release v1.0.0
echo "🌱 Creating Base Release v1.0.0..."
echo "# Synthetic Project v1.0.0" > "${REPO_DIR}/README.md"
mkdir -p "${REPO_DIR}/pkg/api"
echo "package api" > "${REPO_DIR}/pkg/api/api.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "chore: initial v1.0.0 foundation"
git -C "${REPO_DIR}" tag v1.0.0

# Release Delta Commits towards v1.1.0
echo "🚀 Adding feature and fix commits for v1.1.0..."

# Commit 1: Feature
echo "func Login() {}" >> "${REPO_DIR}/pkg/api/api.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(api): add user login endpoint"

# Commit 2: Bug Fix
echo "func RefreshToken() {}" >> "${REPO_DIR}/pkg/api/api.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "fix(auth): resolve session token expiration timeout"

# Commit 3: Breaking Change (Conventional Commits syntax)
echo "func V2Endpoint() {}" >> "${REPO_DIR}/pkg/api/api.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(api)!: migrate endpoint schemas to JSON:API v2 format"

# Commit 4: Sensitive file - CI/CD pipeline (Blast radius)
mkdir -p "${REPO_DIR}/.github/workflows"
echo "name: Deploy" > "${REPO_DIR}/.github/workflows/deploy.yml"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "ci: add automated deployment workflow"

# Commit 5: Sensitive file - Database migration (Breaking change & Blast radius)
mkdir -p "${REPO_DIR}/migrations"
echo "CREATE TABLE accounts (id SERIAL PRIMARY KEY);" > "${REPO_DIR}/migrations/001_accounts.sql"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(db): add account table schema migration"

# Commit 6: Automated tests
mkdir -p "${REPO_DIR}/pkg/api"
echo "package api_test" > "${REPO_DIR}/pkg/api/api_test.go"
echo "func TestLogin() {}" >> "${REPO_DIR}/pkg/api/api_test.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "test(api): add unit tests for login and session"

# Tag Release v1.1.0
echo "🏷️ Tagging v1.1.0..."
git -C "${REPO_DIR}" tag v1.1.0

# Run gh-stats in Release Mode with auto-detection
echo "🔍 Executing gh-stats --target=release (auto-detecting v1.0.0 -> v1.1.0)..."
"${BINARY_PATH}" \
  --target=release \
  --repo-path="${REPO_DIR}" \
  --output="${SARIF_OUT}" \
  --export-json="${JSON_OUT}"

# Validate SARIF Output
echo "📋 Validating generated SARIF report..."
if [ ! -f "${SARIF_OUT}" ]; then
  echo "❌ Error: SARIF report file not created!"
  exit 1
fi

grep -q "GHSTATS201-RELEASE-SUMMARY" "${SARIF_OUT}" || {
  echo "❌ Error: SARIF missing GHSTATS201-RELEASE-SUMMARY rule!"
  exit 1
}

grep -q "GHSTATS202-RELEASE-BREAKING" "${SARIF_OUT}" || {
  echo "❌ Error: SARIF missing GHSTATS202-RELEASE-BREAKING rule!"
  exit 1
}

grep -q "GHSTATS203-RELEASE-BLAST-RADIUS" "${SARIF_OUT}" || {
  echo "❌ Error: SARIF missing GHSTATS203-RELEASE-BLAST-RADIUS rule!"
  exit 1
}

# Validate DORA JSON Output
echo "📊 Validating exported DORA metrics JSON..."
if [ ! -f "${JSON_OUT}" ]; then
  echo "❌ Error: DORA metrics JSON file not created!"
  exit 1
fi

grep -q '"target": "release"' "${JSON_OUT}" || {
  echo "❌ Error: DORA JSON does not specify target=release!"
  exit 1
}

grep -q '"breakingChangesCount":' "${JSON_OUT}" || {
  echo "❌ Error: DORA JSON missing breakingChangesCount!"
  exit 1
}

# Run gh-stats in Range Mode with explicit flags (--target=range --base=v1.0.0 --head=v1.1.0)
echo "🔍 Executing gh-stats --target=range with explicit refs..."
"${BINARY_PATH}" \
  --target=range \
  --repo-path="${REPO_DIR}" \
  --base=v1.0.0 \
  --head=v1.1.0 \
  --output="${TEMP_DIR}/range.sarif" \
  --quiet

if [ ! -f "${TEMP_DIR}/range.sarif" ]; then
  echo "❌ Error: Range SARIF report not created!"
  exit 1
fi

echo "✅ Synthetic Release Comparison Test Passed Successfully!"
