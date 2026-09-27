#!/usr/bin/env bash
set -euo pipefail

# test-synthetic-drift.sh
# End-to-end synthetic test for gh-stats Environment Drift & Promotion Audit (--target=drift).

echo "🧪 Starting Synthetic Environment Drift & Promotion Audit Test..."

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "${TEMP_DIR}"' EXIT

BINARY_PATH="${TEMP_DIR}/gh-stats"
REPO_DIR="${TEMP_DIR}/synthetic-repo"
SARIF_OUT="${TEMP_DIR}/drift.sarif"
JSON_OUT="${TEMP_DIR}/drift.json"

# Build gh-stats binary
echo "🔨 Building gh-stats binary..."
go build -o "${BINARY_PATH}" ./cmd/gh-stats

# Initialize synthetic git repository
echo "📁 Initializing synthetic repository at ${REPO_DIR}..."
mkdir -p "${REPO_DIR}"
git -C "${REPO_DIR}" init -b production
git -C "${REPO_DIR}" config user.name "Synthetic Drift Auditor"
git -C "${REPO_DIR}" config user.email "auditor@example.com"
git -C "${REPO_DIR}" config commit.gpgsign false

# 1. Base commit on production
echo "🌱 Initializing production environment..."
echo "# Production Core Service" > "${REPO_DIR}/README.md"
mkdir -p "${REPO_DIR}/pkg/core"
echo "package core" > "${REPO_DIR}/pkg/core/core.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "chore: initial production baseline"

# 2. Branch to staging
echo "🌿 Creating staging candidate branch..."
git -C "${REPO_DIR}" checkout -b staging

# 3. Test Phase 1: Clean Parity (In-Sync)
echo "🔎 Phase 1: Verifying In-Sync Parity..."
"${BINARY_PATH}" \
  --target=drift \
  --base=production \
  --head=staging \
  --repo-path="${REPO_DIR}" \
  --output="${SARIF_OUT}" \
  --export-json="${JSON_OUT}" \
  --quiet

if ! grep -q '"riskScore": 0' "${JSON_OUT}" && ! grep -q '"riskScore":0' "${JSON_OUT}"; then
  echo "❌ Expected risk score 0 for in-sync branches, got JSON:"
  cat "${JSON_OUT}"
  exit 1
fi
echo "✅ Phase 1 Passed: In-sync environments verified with 0 drift risk"

# 4. Generate candidate unpromoted commits on staging
echo "🚀 Adding unpromoted candidate commits on staging..."

# Commit 1: Feature
echo "func Auth() {}" >> "${REPO_DIR}/pkg/core/core.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(auth): implement user authentication"

# Commit 2: Breaking Change
echo "func V2Contract() {}" >> "${REPO_DIR}/pkg/core/core.go"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(api)!: breaking api v2 contract modification"

# Commit 3: Database Schema Migration
mkdir -p "${REPO_DIR}/migrations"
echo "CREATE TABLE accounts (id SERIAL PRIMARY KEY, name VARCHAR(255));" > "${REPO_DIR}/migrations/001_accounts.sql"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "feat(db): add accounts schema migration"

# Commit 4: Sensitive CI/CD workflow file
mkdir -p "${REPO_DIR}/.github/workflows"
echo "name: CI/CD Pipeline" > "${REPO_DIR}/.github/workflows/deploy.yml"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "ci: add production deployment workflow"

# 5. Add emergency hotfix to production (upstream divergence)
echo "🔥 Adding emergency hotfix directly to production..."
git -C "${REPO_DIR}" checkout production
echo "EMERGENCY_PATCH=true" >> "${REPO_DIR}/patch.env"
git -C "${REPO_DIR}" add .
git -C "${REPO_DIR}" commit -m "fix(prod): hotfix live payment gateway memory leak"

# 6. Test Phase 2: Audit Diverged Environment Drift
echo "🔎 Phase 2: Auditing Diverged Environment Drift (production ➔ staging)..."
"${BINARY_PATH}" \
  --target=drift \
  --base=production \
  --head=staging \
  --repo-path="${REPO_DIR}" \
  --output="${SARIF_OUT}" \
  --export-json="${JSON_OUT}"

# Validate JSON metrics
echo "📊 Validating DORA & Drift telemetry JSON..."
grep -q '"target": "drift"' "${JSON_OUT}" || grep -q '"target":"drift"' "${JSON_OUT}"
grep -q '"commitsAhead": 4' "${JSON_OUT}" || grep -q '"commitsAhead":4' "${JSON_OUT}"
grep -q '"commitsBehind": 1' "${JSON_OUT}" || grep -q '"commitsBehind":1' "${JSON_OUT}"
grep -q '"breakingChangesCount": 2' "${JSON_OUT}" || grep -q '"breakingChangesCount":2' "${JSON_OUT}"
grep -q '"sensitiveFilesCount": 2' "${JSON_OUT}" || grep -q '"sensitiveFilesCount":2' "${JSON_OUT}"

# Validate SARIF report
echo "🛡️ Validating SARIF rules and results..."
grep -q "GHSTATS301-DRIFT-SUMMARY" "${SARIF_OUT}"
grep -q "GHSTATS303-DRIFT-UNPROMOTED-BREAKING" "${SARIF_OUT}"
grep -q "GHSTATS304-DRIFT-UNPROMOTED-SENSITIVE" "${SARIF_OUT}"

# 7. Test Phase 3: Quality Gate Enforcement (--fail-on)
echo "🚫 Phase 3: Testing Quality Gate Enforcement (--fail-on=HIGH)..."
set +e
"${BINARY_PATH}" \
  --target=drift \
  --base=production \
  --head=staging \
  --repo-path="${REPO_DIR}" \
  --fail-on=HIGH \
  --quiet
FAIL_EXIT=$?
set -e

if [ "${FAIL_EXIT}" -ne 2 ]; then
  echo "❌ Expected exit code 2 for --fail-on=HIGH on high risk drift, got: ${FAIL_EXIT}"
  exit 1
fi
echo "✅ Phase 3 Passed: Quality gate correctly blocked promotion with exit code 2"

echo "🎉 All Synthetic Environment Drift Tests Passed Successfully!"
