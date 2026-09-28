# ⚡ gh-stats

A production-grade GitHub Action and CLI tool written in Go that generates deep **SRE & developer reliability statistics** for Pull Requests or entire repositories, outputting standards-compliant **SARIF v2.1.0** reports directly into GitHub Code Scanning.

🌐 **Documentation & Live Playground:** [smford.github.io/gh-stats](https://smford.github.io/gh-stats/)

---

## 🎯 Why gh-stats? (An SRE & Senior Developer Perspective)

In high-velocity engineering organizations, code reviews often suffer from two major operational problems:
1. **Hidden Blast Radius & Review Fatigue:** PRs modifying critical deployment pipelines (`.github/workflows`), Kubernetes configurations, Terraform files, or database migrations are easily merged without adequate scrutiny.
2. **Missing Test Delta:** Large volumes of business logic are committed without corresponding automated test changes, causing test coverage erosion and elevated Mean Time to Recovery (MTTR).

`gh-stats` acts as an automated reliability and architectural gate:
- 🛡️ **Calculates PR Risk Scores (0–100):** Evaluates change size, cognitive load, test-to-code ratios, and blast radius.
- 🚨 **Pinpoints High-Blast-Radius Changes:** Flags changes touching CI/CD, infrastructure-as-code, auth, and database schemas.
- 👥 **Intelligent Reviewer Routing:** Analyzes historical git logs for modified files to recommend component domain experts.
- 🔍 **Surfaces Code Churn Hotspots (Repo Mode):** Detects frequently modified files that correlate with regression incidents.
- 👥 **Identifies Bus Factor Risk:** Highlights contributor concentration to mitigate domain knowledge silos.
- 📋 **Seamless SARIF & Job Summary Integration:** Ingests into GitHub Code Scanning and renders formatted markdown tables in `$GITHUB_STEP_SUMMARY`.

---

## 🏗️ Architecture

```
                  ┌─────────────────────────────────────────┐
                  │           gh-stats Engine (Go)          │
                  └────────────────────┬────────────────────┘
                                       │
                ┌──────────────────────┴──────────────────────┐
                │                                             │
      ┌─────────▼─────────┐                         ┌─────────▼─────────┐
      │   PR Mode (Diff)  │                         │  Repo Mode (Log)  │
      ├───────────────────┤                         ├───────────────────┤
      │ • Lines Added/Del │                         │ • Churn Hotspots  │
      │ • Test Delta      │                         │ • Bus Factor      │
      │ • Blast Radius    │                         │ • Test Density    │
      │ • SRE Risk Score  │                         │ • File Inventory  │
      └─────────┬─────────┘                         └─────────┬─────────┘
                │                                             │
                └──────────────────────┬──────────────────────┘
                                       │
                     ┌─────────────────┴─────────────────┐
                     │                                   │
           ┌─────────▼─────────┐               ┌─────────▼─────────┐
           │ SARIF v2.1.0 File │               │ Step Summary (MD) │
           │ (Code Scanning)   │               │ (Actions Tab)     │
           └───────────────────┘               └───────────────────┘
```

- **Zero Heavy External Dependencies:** Built using Go standard library and native git commands for fast execution (<2 seconds) and minimal supply-chain risk.
- **OASIS SARIF 2.1.0 Compliant:** Works natively with GitHub's CodeQL and Code Scanning ingestion pipeline.

---

## 🚀 Quickstart: GitHub Actions Setup

### 1. Pull Request Workflow (`.github/workflows/pr-stats.yml`)

Add this workflow to analyze every PR and publish code scanning annotations:

```yaml
name: PR Reliability & Stats

on:
  pull_request:
    branches: [ main, master ]

permissions:
  contents: read
  security-events: write   # Required to upload SARIF to Code Scanning
  pull-requests: write     # Required to post/update sticky comments on the PR

jobs:
  gh-stats:
    name: SRE PR Analysis & Quality Gate
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0 # Full history required for diff analysis

      - name: Run gh-stats Action
        uses: smford/gh-stats@main
        with:
          target: pr
          output: gh-stats.sarif
          upload-sarif: 'true'
          category: 'gh-stats-pr'
          comment-pr: 'true'   # Posts live sticky comment on PR
          fail-on: 'CRITICAL'  # Quality gate: blocks PR if risk is CRITICAL
          token: ${{ secrets.GITHUB_TOKEN }}
```

### 2. Repository Health Workflow (`.github/workflows/repo-stats.yml`)

Run periodically or on main branch push to detect churn hotspots and contributor distribution:

```yaml
name: Repository Architecture & Health

on:
  push:
    branches: [ main ]
  schedule:
    - cron: '0 0 * * 1' # Weekly Monday midnight run

permissions:
  contents: read
  security-events: write

jobs:
  repo-stats:
    name: Repo SRE Health
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run gh-stats
        uses: smford/gh-stats@main
        with:
          target: repo
          commit-limit: '250'
          output: repo-stats.sarif
          upload-sarif: 'true'
          category: 'gh-stats-repo'
```

---

## ⚙️ Action Inputs & Outputs

### Inputs

| Input | Description | Default | Required |
| :--- | :--- | :---: | :---: |
| `target` | Analysis mode: `'pr'`, `'repo'`, `'release'`, `'range'`, `'drift'`, or `'auto'` (auto-detects based on event) | `'auto'` | No |
| `base-ref` | Base git reference for PR, release, or drift comparison (defaults to auto-detected previous tag in release mode, base branch in PR mode, or production branch in drift mode) | `github.base_ref` | No |
| `head-ref` | Head git reference for PR, release, or drift comparison (defaults to HEAD, latest tag in release mode, or staging branch in drift mode) | `HEAD` | No |
| `output` | Destination file path for generated SARIF report | `gh-stats.sarif` | No |
| `commit-limit` | Maximum commit history to analyze in `repo` mode | `200` | No |
| `upload-sarif` | Automatically upload SARIF to GitHub Code Scanning via `@actions/upload-sarif` | `'true'` | No |
| `category` | SARIF category label in GitHub Code Scanning | `gh-stats` | No |
| `comment-pr` | Post or update a live sticky Markdown summary comment on the PR conversation thread | `'false'` | No |
| `fail-on` | Enforce risk budget gating: fail job if PR, release, or drift risk meets/exceeds threshold (`'CRITICAL'`, `'HIGH'`, `'MEDIUM'`, `'LOW'`) | `''` (disabled) | No |
| `config-path` | Path to `.gh-stats.yml` configuration file (auto-discovers `.gh-stats.yml` or `.github/.gh-stats.yml` if omitted) | `''` | No |
| `export-json` | File path to export structured DORA & SRE metrics in JSON format | `''` (disabled) | No |
| `export-prom` | File path to export Prometheus text exposition format metrics (`.prom`) | `''` (disabled) | No |
| `export-otel-endpoint` | OpenTelemetry OTLP HTTP metrics endpoint URL (e.g. `http://otel-collector:4318/v1/metrics`) | `''` (disabled) | No |
| `export-otel-headers` | Custom HTTP headers for OTLP export (e.g. `X-Scope-OrgID=123,Authorization=Bearer ...`) | `''` | No |
| `export-webhook` | HTTP/HTTPS Webhook endpoint to dispatch DORA & SRE metrics payload | `''` (disabled) | No |
| `webhook-secret` | HMAC-SHA256 signature secret or Bearer token for webhook authentication | `''` | No |
| `token` | GitHub token for authentication (API stats, sticky comment, SARIF upload) | `${{ github.token }}` | No |

### Outputs

| Output | Description |
| :--- | :--- |
| `sarif-file` | Path to the generated SARIF report file |
| `metrics-json` | Path to the exported metrics JSON file (if `export-json` was enabled) |
| `metrics-prom` | Path to the exported Prometheus metrics file (if `export-prom` was enabled) |
| `otel-status` | Dispatch status (`success` or `failure`) of OpenTelemetry OTLP export |
| `release-base-ref` | Base git reference used in release comparison mode |
| `release-head-ref` | Head git reference used in release comparison mode |
| `release-commits-count` | Total commit count in the release delta |
| `release-breaking-count` | Number of breaking changes and database migrations detected in the release |
| `release-risk-level` | Evaluated deployment risk rating (`LOW`, `MEDIUM`, `HIGH`, `CRITICAL`) |
| `release-risk-score` | Numerical deployment risk score (0-100) |
| `drift-base-ref` | Target environment reference evaluated in drift mode |
| `drift-head-ref` | Candidate environment reference evaluated in drift mode |
| `drift-commits-ahead` | Number of unpromoted commits in candidate environment awaiting promotion |
| `drift-commits-behind` | Number of diverged/missing upstream commits in candidate environment |
| `drift-breaking-count` | Number of unpromoted breaking changes or schema migrations detected |
| `drift-sensitive-count` | Number of unpromoted sensitive infrastructure, CI/CD, or security files |
| `drift-risk-level` | Evaluated promotion deployment risk rating (`LOW`, `MEDIUM`, `HIGH`, `CRITICAL`) |
| `drift-risk-score` | Numerical promotion deployment risk score (0-100) |
| `target` | Resolved analysis target (`pr`, `repo`, `release`, or `drift`) |

---

## 🏷️ Generated SARIF Rules & Alerts

| Rule ID | Severity | Scope | Condition |
| :--- | :---: | :---: | :--- |
| `GHSTATS001-PR-SUMMARY` | `note` / `warning` | PR | Overall SRE health scorecard, lines added/deleted, file distribution |
| `GHSTATS002-PR-SIZE` | `warning` | PR | PR exceeds 800 lines changed (review fatigue & MTTD risk) |
| `GHSTATS003-PR-TEST-RATIO` | `warning` | PR | >80 lines of production code changed with zero automated test delta |
| `GHSTATS004-PR-BLAST-RADIUS` | `warning` | PR | Critical file modified (CI/CD workflows, Terraform/k8s, DB migrations, lockfiles) |
| `GHSTATS005-PR-STALE` | `warning` | PR | PR has remained open >14 days (stale branch / merge drift risk) |
| `GHSTATS006-PR-DISCUSSION-CHURN` | `note` | PR | High comment volume (>15 comments) indicating review friction / ambiguity |
| `GHSTATS007-PR-REVIEWERS` | `note` | PR | Historical domain experts recommended to review modified files |
| `GHSTATS008-PR-CI-LATENCY` | `warning` | PR | CI pipeline check run exceeds latency threshold |
| `GHSTATS009-PR-CI-FLAKINESS` | `warning` | PR | CI check run failed and then passed on retry on the same commit SHA |
| `GHSTATS101-REPO-SUMMARY` | `note` | Repo | Architecture overview, test density %, language breakdown |
| `GHSTATS102-REPO-HOTSPOTS` | `note` | Repo | High-churn files identified across commit history |
| `GHSTATS103-REPO-BUS-FACTOR` | `warning` | Repo | Single contributor accounts for >75% of commits |
| `GHSTATS104-REPO-API-METADATA` | `note` | Repo | GitHub ecosystem metrics (stars, forks, open issue queue) |
| `GHSTATS201-RELEASE-SUMMARY` | `note` | Release | Release comparison delta, velocity, breaking change count, and readiness summary |
| `GHSTATS202-RELEASE-BREAKING` | `warning` | Release | Breaking change syntax (`!:`, `BREAKING CHANGE:`) or database schema migrations detected |
| `GHSTATS203-RELEASE-BLAST-RADIUS` | `warning` | Release | High-blast-radius infrastructure, CI/CD, or auth files modified in release |
| `GHSTATS301-DRIFT-SUMMARY` | `note` | Drift | Environment drift and promotion audit summary, ahead/behind counts, and unpromoted volume |
| `GHSTATS302-DRIFT-EXCESSIVE` | `warning` | Drift | Excessive commit divergence detected between deployment environments (batch promotion risk) |
| `GHSTATS303-DRIFT-UNPROMOTED-BREAKING` | `warning` | Drift | Unpromoted breaking changes or database schema migrations waiting between environments |
| `GHSTATS304-DRIFT-UNPROMOTED-SENSITIVE` | `warning` | Drift | Unpromoted critical infrastructure, CI/CD pipeline, or security configuration changes |

---

## ⚙️ Configuration as Code (`.gh-stats.yml`)

Repositories can customize risk thresholds, define custom blast radius patterns, and ignore paths using a `.gh-stats.yml` or `.gh-stats.yaml` file located in the repository root or `.github/` folder:

```yaml
# .gh-stats.yml

# Custom threshold limits for risk scoring and alerts
thresholds:
  max_pr_lines: 800        # Lines of change triggering large PR alert (default: 800)
  stale_pr_days: 14        # PR age in days triggering stale branch drift alert (default: 14)
  min_test_ratio: 0.2      # Minimum ratio of test lines to production code (default: 0.2)
  max_discussions: 15      # Discussion comments threshold for review friction alert (default: 15)
  commit_limit: 200        # Commit window for repository churn and hotspot analysis (default: 200)

# Custom blast radius patterns (glob syntax with '**' recursive support)
blast_radius:
  custom_patterns:
    - category: "Billing Engine"
      pattern: "services/billing/**"
      description: "Modifications to revenue, invoicing, or payment processing pipelines"
    - category: "Database Models"
      pattern: "models/**"
      description: "Core database schema and ORM entity definitions"

# Paths or globs to ignore completely from diff line counts and blast radius checks
ignore:
  paths:
    - "vendor/**"
    - "**/*.pb.go"
    - "**/*_gen.go"
    - "frontend/dist/**"

# Default quality gate threshold: fails workflow if PR risk meets or exceeds this level
# Options: LOW, MEDIUM, HIGH, CRITICAL
fail_on: "HIGH"

# DORA & SRE Observability Telemetry Exporter
export:
  json_path: "metrics.json"
  prom_path: "metrics.prom"
  otel_endpoint: "http://otel-collector:4318/v1/metrics"
  otel_headers:
    Authorization: "Bearer secret-token"
    X-Scope-OrgID: "engineering"
  webhook_url: "https://metrics.internal/v1/dora"
  webhook_secret: "secret-token"
```

---

## 📊 DORA & SRE Observability Exporter (Prometheus, OpenTelemetry & Webhook)

Track reliability velocity and correlate risk with downstream deployments:
- **Lead Time for Changes:** PR lifecycle duration from creation to review and merge.
- **Deployment & Promotion Risk Scoring:** Numerical risk score (0-100) and risk level across PRs, Releases, and Environment Drift.
- **Change Failure Rate Correlation:** Quantify risk scores against production rollback/incident frequencies.
- **Test Debt Velocity:** Track test-to-code ratio trends over time across squads.
- **CI Pipeline Observability:** Track cumulative check run duration, bottleneck times, and flaky test occurrences.
- **Zero-Dependency Native Exporters:**
  - **Prometheus Text Exposition Format (`.prom`):** Standard `# HELP` and `# TYPE` gauges and counters compatible with Prometheus textfile collector, Pushgateway, Grafana Mimir, VictoriaMetrics, and Datadog Prometheus agent.
  - **Native OpenTelemetry (OTel) OTLP HTTP Export:** Dispatches standard OTLP JSON metrics over HTTP directly to OpenTelemetry Collector (`http://otel-collector:4318/v1/metrics`), Grafana Cloud, Datadog OTLP ingest, or Honeycomb with custom headers.
  - **JSON & Webhook Dispatch:** Push structured JSON telemetry with optional HMAC-SHA256 verification (`X-Hub-Signature-256`) and Bearer authentication.

### CLI Usage Examples

```bash
# Export Prometheus text exposition format
gh-stats -target=pr -export-prom=metrics.prom

# Export OpenTelemetry metrics over OTLP HTTP
gh-stats -target=pr \
  -export-otel-endpoint=http://otel-collector:4318/v1/metrics \
  -export-otel-headers="Authorization=Bearer my-token,X-Scope-OrgID=prod"

# Export metrics to a JSON file
gh-stats -target=pr -export-json=metrics.json

# Stream metrics to an observability webhook
gh-stats -target=pr -export-webhook=https://metrics.internal/v1/dora -webhook-secret="secret-token"
```

### Ingestion Examples

#### 1. Prometheus & Pushgateway (Grafana Dashboard)
Push exported Prometheus metrics to a Prometheus Pushgateway or expose via textfile collector for Prometheus/Mimir scraping:

```yaml
- name: Run gh-stats and Export Prometheus Metrics
  uses: smford/gh-stats@main
  with:
    target: pr
    export-prom: metrics.prom
    fail-on: HIGH

- name: Push to Prometheus Pushgateway
  if: always()
  run: |
    if [ -f metrics.prom ]; then
      curl --data-binary @metrics.prom \
        "http://pushgateway.monitoring.svc:9091/metrics/job/gh-stats/instance/${{ github.repository }}"
    fi
```

**Prometheus Metrics Emitted:**
| Metric | Type | Description |
| :--- | :--- | :--- |
| `gh_stats_pr_risk_score` | Gauge | SRE risk score (0-100) for the PR |
| `gh_stats_pr_lead_time_seconds` | Gauge | Pull request lifecycle lead time in seconds |
| `gh_stats_pr_lines_added_total` | Counter | Total lines added in the PR delta |
| `gh_stats_pr_lines_deleted_total` | Counter | Total lines deleted in the PR delta |
| `gh_stats_pr_lines_net` | Gauge | Net lines changed in the PR delta |
| `gh_stats_ci_latency_seconds` | Gauge | Total cumulative CI check runs duration |
| `gh_stats_ci_flaky_checks_total` | Counter | Number of flaky check runs detected |
| `gh_stats_release_risk_score` | Gauge | Release deployment risk score (0-100) |
| `gh_stats_release_breaking_changes_total` | Counter | Breaking changes & migrations in release |
| `gh_stats_drift_risk_score` | Gauge | Promotion deployment risk score (0-100) |
| `gh_stats_drift_commits_ahead` | Gauge | Candidate commits awaiting promotion |
| `gh_stats_drift_commits_behind` | Gauge | Missing upstream commits behind target |

#### 2. OpenTelemetry (OTel Collector & Grafana Cloud)
Dispatch OTLP JSON directly to an OpenTelemetry Collector or Grafana Cloud OTLP Gateway:

```yaml
- name: Run gh-stats with OpenTelemetry Export
  uses: smford/gh-stats@main
  with:
    target: release
    export-otel-endpoint: "https://otlp-gateway-prod-us-east-0.grafana.net/otlp/v1/metrics"
    export-otel-headers: "Authorization=Basic ${{ secrets.GRAFANA_OTLP_TOKEN }}"
```

#### 3. Datadog Ingestion
Stream OTLP metrics directly to Datadog's OTLP HTTP intake endpoint or via a local Datadog Agent:

```yaml
- name: Run gh-stats with Datadog OTLP Ingest
  uses: smford/gh-stats@main
  with:
    target: pr
    export-otel-endpoint: "https://otlp.datadoghq.com/v1/metrics"
    export-otel-headers: "dd-api-key=${{ secrets.DATADOG_API_KEY }}"
```

---

## ⚡ CI Pipeline Latency & Flakiness Detection (GitHub Check Runs API)

Slow and flaky CI pipelines are among the highest sources of developer friction, context-switching overhead, and deployment delays. When tests intermittently fail, developers are trained to "re-run until green," masking legitimate regressions.

`gh-stats` integrates with the **GitHub Check Runs API** (`GET /repos/{owner}/{repo}/commits/{ref}/check-runs?filter=all`) to provide automated CI observability and quality gates:

- **Pipeline Turnaround & Critical Path Bottlenecks:** Tracks individual job execution times, calculates cumulative pipeline latency, and pinpoints the slowest bottleneck job setting the minimum CI turnaround time.
- **Definitive Flakiness Detection:** Identifies check runs that were re-run on the exact same commit SHA and exhibited conflicting outcomes (e.g. failed on run 1, passed on retry).
- **Automated Quality Gates:** Fail workflows when flaky checks occur (`fail-on-flaky: true` or `--fail-on-flaky`) or when individual jobs exceed latency thresholds (`--max-ci-latency=15`).
- **SARIF Code Scanning Alerts:**
  - `GHSTATS008-PR-CI-LATENCY`: Flags checks exceeding the maximum latency threshold.
  - `GHSTATS009-PR-CI-FLAKINESS`: Flags tests with conflicting outcomes across retries on the same commit.

### Sample Step Summary Output:

```markdown
## ⚡ CI Pipeline Latency & Reliability (Check Runs)

| Metric | Value |
| :--- | :--- |
| **Check Runs Evaluated** | `5` total (`4` passed, `1` failed) |
| **Cumulative CI Runtime** | `24m 12s` |
| **Critical Path Bottleneck** | `E2E Cypress` (`18m 20s`) |
| **Flaky Checks** | `🚨 1 detected` |

### ⚠️ Flaky & Retried Checks
| Check Run | Retries | Initial Outcome | Final Outcome | Verdict |
| :--- | :---: | :---: | :---: | :--- |
| `Unit Tests` | 1 | `failure` | `success` | 🚨 **Flaky** (Passed on retry) |
```

---

## 📦 Release Comparison Mode (`--target=release` or `--target=range`)

Preparing a release candidate or deploying a milestone requires understanding the full scope of changes between tags. Without automated delta analysis, teams often ship silent breaking changes, undocumented migrations, or infrastructure alterations.

`gh-stats` provides a specialized **Release Comparison Mode** that evaluates the exact delta between two release milestones:

- **Smart Tag Auto-Detection:**
  - Running `gh-stats --target=release` automatically identifies the latest two SemVer release tags (e.g. `v0.3.0` and `v0.4.0`) and calculates the delta.
  - Specify custom releases or branches via `--base` and `--head` (e.g. `--base=v0.2.0 --head=v0.4.0` or `--base=v0.4.0 --head=HEAD`).
  - `--target=range` is fully supported as an intuitive alias.
- **Breaking Change & Schema Migration Detection:**
  - Identifies Conventional Commits breaking changes (`feat!:`, `fix!:`, or `BREAKING CHANGE:`).
  - Flags database schema migration files (`*.sql` or `migrations/`).
- **Velocity & Test Delta:**
  - Tracks lines added, lines deleted, net code volume, and test-to-code velocity ratio across the release window.
- **Categorized Release Notes / Changelog:**
  - Automatically groups commits into Features (🚀), Bug Fixes (🐛), Performance (⚡), Refactoring (🛠️), and Chores.
- **Deployment Risk Scoring & Quality Gate:**
  - Rates release risk from `LOW` to `CRITICAL` based on breaking changes, migration presence, volume, and blast radius.
  - Blocks deployment pipelines using `--fail-on=HIGH` or `--fail-on=CRITICAL`.
- **DORA Observability:** Exports structured release metrics to JSON or observability webhooks.

### Sample Release Step Summary:

```markdown
# 📦 GitHub Stats: Release Comparison (`v0.3.0...v0.4.0`)

> **Release Deployment Risk:** `LOW` (Score: **20 / 100**) 🛡️

## 📊 Release Delta Overview

| Metric | Value |
| :--- | :--- |
| **Comparison Range** | `v0.3.0` ... `v0.4.0` |
| **Commits** | `3` |
| **Contributors** | `2` unique author(s) |
| **Lines Added** | `+885` |
| **Lines Deleted** | `-25` |
| **Net Delta** | `+860` lines |
| **Files Modified** | `17` (Code: `10`, Tests: `5`, Docs: `2`) |
| **Test vs Code Delta** | `+245` test lines / `+554` code lines (44.2%) |

## 📝 Release Changelog

### 🚀 Features
- [`959165b`] feat: CI Pipeline Latency & Flakiness Detection (@smford)
```

### GitHub Actions Release Workflow (`.github/workflows/release-stats.yml`)

```yaml
name: Release Readiness & Health

on:
  release:
    types: [ published ]
  workflow_dispatch:
    inputs:
      base_tag:
        description: 'Base release tag (auto-detected if blank)'
        required: false
      head_tag:
        description: 'Head release tag (defaults to current release/HEAD)'
        required: false

permissions:
  contents: read
  security-events: write

jobs:
  release-audit:
    name: Release SRE Readiness
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run gh-stats Release Comparison
        uses: smford/gh-stats@main
        with:
          target: release
          base-ref: ${{ inputs.base_tag }}
          head-ref: ${{ inputs.head_tag }}
          output: release-stats.sarif
          fail-on: 'CRITICAL'
          export-json: release-metrics.json
```

---

## 🏷️ Deterministic Semantic Version Bump (`--suggest-bump`)

Eliminate guesswork and manually maintained release scripts. `gh-stats` deterministically inspects Conventional Commits and file deltas to recommend the next Semantic Version bump (`major`, `minor`, `patch`) and next release tag:

1. **`MAJOR` Bump:**
   - Conventional Commit breaking changes (`feat!:`, `fix!:`, `refactor!:`, or `BREAKING CHANGE:` in commit subject/body).
   - Database schema migrations (`migrations/`, `*.sql`).
2. **`MINOR` Bump:**
   - New features introduced (`feat:`, `feat(...)`) without breaking changes.
3. **`PATCH` Bump:**
   - Bug fixes (`fix:`), performance optimizations (`perf:`), documentation (`docs:`), chores (`chore:`), or test additions.
4. **`NONE`:**
   - No code commits or file differences detected across the comparison window.

### CLI Usage:

```bash
# Recommend next bump and calculate version from latest release tag to HEAD
gh-stats --suggest-bump

# Explicitly evaluate release delta against latest release
gh-stats --target=release --suggest-bump

# Evaluate a specific tag or milestone range
gh-stats --target=release --base=v0.4.0 --head=HEAD --suggest-bump
```

Output:
```text
🔍 gh-stats: running in [RELEASE] mode
📦 Comparing Release: v0.4.0 ... HEAD
✅ Release Analysis Complete: Risk=MEDIUM (40/100), Commits=3, Breaking Changes=0, Files=7
🏷️ Suggested Version Bump: MINOR (current: v0.4.0 ➔ next: v0.5.0)
```

### GitHub Action Integration:

```yaml
- name: Determine Next Release Version
  id: semver
  uses: smford/gh-stats@main
  with:
    target: release
    suggested-bump: 'true'

- name: Tag and Release
  if: steps.semver.outputs.suggested-bump != 'none'
  run: |
    echo "Recommended Bump: ${{ steps.semver.outputs.suggested-bump }}"
    echo "Next Tag: ${{ steps.semver.outputs.suggested-version }}"
    git tag -a "${{ steps.semver.outputs.suggested-version }}" -m "Release ${{ steps.semver.outputs.suggested-version }}"
    git push origin "${{ steps.semver.outputs.suggested-version }}"
```

### GitHub Action Outputs:

| Output | Description | Example |
| :--- | :--- | :--- |
| `suggested-bump` | Recommended SemVer bump (`major`, `minor`, `patch`, or `none`) | `minor` |
| `suggested-version` | Next calculated SemVer release tag | `v0.5.0` |

## 🌐 Environment Drift & Promotion Audit (`--target=drift`)

In continuous delivery architectures with multiple deployment tiers (e.g. `staging` ➔ `production`, or `dev` ➔ `qa`), long-lived branch divergence leads to high-risk batch promotions, unpromoted database migrations, and release pipeline failures.

`gh-stats` provides **Environment Drift & Promotion Audit Mode** (`--target=drift`) to quantify deployment readiness and risk between environments before triggering a promotion:

- **Divergence Velocity (Commits Ahead & Behind):**
  - Evaluates how many commits candidate environment is **ahead** of target base (unpromoted commits queued for release).
  - Evaluates how many commits candidate environment is **behind** target base (upstream divergence or emergency production hotfixes not yet back-merged).
- **Unpromoted Blast Radius & Breaking Changes:**
  - Flags unpromoted database schema migrations (`migrations/`, `*.sql`).
  - Pinpoints unpromoted infrastructure, container, security, or CI/CD workflow alterations.
  - Detects unpromoted breaking changes from Conventional Commits (`feat!:`, `BREAKING CHANGE:`).
- **Promotion Deployment Risk Score (0-100):**
  - Computes a deterministic SRE risk score and qualitative risk tier (`LOW`, `MEDIUM`, `HIGH`, `CRITICAL`).
  - Enforces automated quality gating via `--fail-on=HIGH` or `--fail-on=CRITICAL` to halt risky promotions.
- **DORA & Observability Export:**
  - Emits telemetry with commit divergence and sensitive change counts for DORA dashboards.

### Sample Step Summary Table:

```markdown
# 🌐 Environment Drift & Promotion Assessment

> **Promotion Deployment Risk:** `HIGH` (Score: **65 / 100**) 🛡️

## 🌐 Environment Drift & Promotion Assessment

| Metric | Target / Environment | Status |
| :--- | :--- | :--- |
| **Base Environment (Target)** | `origin/production` | Target Base |
| **Head Environment (Candidate)** | `origin/staging` | Promotion Source |
| **Commits Ahead (Unpromoted)** | `8` commit(s) awaiting promotion | ℹ️ Pending Promotion |
| **Commits Behind (Divergence)** | `2` commit(s) missing from candidate | ℹ️ Pending Promotion |
| **Lines Added** | `+450` | Unpromoted delta |
| **Lines Deleted** | `-30` | Unpromoted delta |
| **Net Change** | `+420` lines | Net code movement |
| **Files Modified** | `6` (Code: `4`, Tests: `1`, Docs: `1`) | Unpromoted files |

## ⚠️ Unpromoted Breaking Changes & Migrations

| Commit / Item | Description | Type |
| :--- | :--- | :--- |
| `c0ffee1` | feat!: breaking API v2 overhaul | `conventional_commit` |
| Schema | Database Migration: migrations/002_orders.sql (+30/-0) | `migration_file` |
```

### GitHub Actions Promotion Audit Workflow (`.github/workflows/environment-drift.yml`)

Run before promoting `staging` to `production` or on a scheduled audit cron:

```yaml
name: Environment Drift & Promotion Audit

on:
  schedule:
    - cron: '0 8 * * 1-5' # Weekday morning audit at 8 AM UTC
  workflow_dispatch:
    inputs:
      base_env:
        description: 'Target promotion environment branch'
        required: true
        default: 'origin/production'
      candidate_env:
        description: 'Candidate environment branch to promote'
        required: true
        default: 'origin/staging'

permissions:
  contents: read
  security-events: write

jobs:
  audit-drift:
    name: Audit Environment Parity
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0 # Full history required for ahead/behind calculation

      - name: Run gh-stats Drift Audit
        uses: smford/gh-stats@main
        with:
          target: drift
          base-ref: ${{ inputs.base_env || 'origin/production' }}
          head-ref: ${{ inputs.candidate_env || 'origin/staging' }}
          fail-on: 'HIGH' # Blocks promotion if drift risk is HIGH or CRITICAL
          output: drift-audit.sarif
          export-json: drift-metrics.json
```

### CLI Usage:

```bash
# Audit drift between production and staging
gh-stats --target=drift --base=origin/production --head=origin/staging

# Audit drift with strict quality gate
gh-stats --target=drift --base=origin/production --head=origin/staging --fail-on=HIGH
```

---

## 🚀 Automated GitHub Release Notes Publishing (`--publish-release-notes`)

Automatically generate and publish production-grade GitHub Release notes directly to GitHub Releases using the GitHub REST API (`POST /repos/{owner}/{repo}/releases` or `PATCH /repos/{owner}/{repo}/releases/{id}`):

- **SRE Deployment Readiness:** Publishes overall release risk level, risk score, and quality gate results.
- **Categorized Changelog:** Groups commits by Conventional Commits (Features 🚀, Bug Fixes 🐛, Performance ⚡, Refactoring 🛠️) with author mentions.
- **Breaking Changes & Migrations Warning:** Highlights breaking changes syntax and database schema modifications (`*.sql`, `migrations/`).
- **Blast Radius Isolation:** Identifies changes to CI/CD workflows, infrastructure manifests (Terraform, Kubernetes), and authentication modules.
- **Release Contributor Roster:** Acknowledges all contributors to the release with commit counts and contribution share.
- **Non-Destructive Updating:** If a release already exists (e.g. created during tag publishing or asset uploads), `gh-stats` preserves existing custom notes and binary asset download links while updating the SRE summary section.

### CLI Usage:

```bash
# Auto-detect latest release tag and publish/update notes
gh-stats --publish-release-notes --token="$GITHUB_TOKEN"

# Publish notes for a specific release tag
gh-stats --target=release --release-tag=v0.6.0 --publish-release-notes --token="$GITHUB_TOKEN"
```

### GitHub Actions Release Workflow:

```yaml
name: Release & Publish

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  release-notes:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout Code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Publish SRE Release Notes
        uses: smford/gh-stats@main
        with:
          target: release
          publish-release-notes: 'true'
          release-tag: ${{ github.ref_name }}
          token: ${{ secrets.GITHUB_TOKEN }}
```

---

## 💻 Local CLI Usage

You can build and run `gh-stats` locally on any git repository:

```bash
# Build
go build -o gh-stats ./cmd/gh-stats

# Analyze repository architecture
./gh-stats -target=repo -output=repo.sarif

# Analyze a feature branch PR against main
./gh-stats -target=pr -base=main -head=HEAD -output=pr.sarif

# Compare latest two releases automatically
./gh-stats -target=release

# Compare a specific release range
./gh-stats -target=range -base=v0.2.0 -head=v0.4.0

# Use a custom configuration file
./gh-stats -target=pr -config=.github/.gh-stats.yml
```

### 🍺 Homebrew Installation (macOS & Linux)

Install `gh-stats` via Homebrew from the official tap:

```bash
brew tap smford/homebrew-tap
brew install gh-stats

# Verify installation
gh-stats -version
```

### CLI Flags:
```text
  -target string
        Target scope: 'pr', 'repo', 'release'/'range', 'drift', or 'auto' (default "auto")
  -base string
        Base ref for PR, release, or drift comparison (e.g. origin/main, v0.3.0, or origin/production)
  -head string
        Head ref for PR, release, or drift comparison (default "HEAD")
  -config string
        Path to .gh-stats.yml configuration file
  -output string
        Path to output SARIF file (default "gh-stats.sarif")
  -repo-path string
        Path to git repository (default ".")
  -commit-limit int
        Maximum commit history to examine for repo hotspots (default 200)
  -fail-on string
        Fail workflow if PR, release, or drift risk meets/exceeds threshold (e.g. 'HIGH', 'CRITICAL')
  -comment-pr
        Post or update a sticky summary comment on the PR
  -export-json string
        Path to export DORA & SRE metrics JSON file
  -export-prom string
        Path to export Prometheus text exposition format metrics file
  -export-otel-endpoint string
        OpenTelemetry OTLP HTTP metrics endpoint URL (e.g. http://otel-collector:4318/v1/metrics)
  -export-otel-headers string
        Custom headers for OTLP HTTP metrics export (key=value,...)
  -export-webhook string
        Webhook URL to export DORA & SRE metrics
  -webhook-secret string
        Secret key or bearer token for webhook export
  -max-ci-latency int
        Maximum acceptable CI check latency in minutes (default 15)
  -fail-on-flaky
        Fail quality gate if flaky CI checks are detected
  -check-runs
        Fetch CI check runs for latency and flakiness analysis (default true)
  -suggest-bump
        Deterministically recommend next semantic version bump and version string
  -publish-release-notes
        Publish or update GitHub release notes with generated SRE summary
  -release-tag string
        GitHub Release tag to publish or update notes for (auto-detected if omitted)
  -version
        Print gh-stats version and exit
  -quiet
        Suppress stdout output
```

---

## 🪝 Shift-Left Local Git Hooks (Pre-Push & Pre-Commit)

Shift reliability and quality gates directly to engineer workstations **before** pushing to remote CI/CD or opening a pull request.

`gh-stats` provides a built-in hook manager that installs lightweight, portable git hooks into your `.git/hooks` directory:

```bash
# Install shift-left pre-push hook (default)
gh-stats hook install

# Specify custom risk threshold for blocking pushes
gh-stats hook install --fail-on=HIGH

# Install pre-commit hook for staged changes
gh-stats hook install --type=pre-commit --fail-on=CRITICAL

# Remove installed hook
gh-stats hook uninstall --type=pre-push
```

### How the Pre-Push Hook Works
1. Runs automatically on `git push`.
2. Compares local commits against the upstream base branch (`origin/main`, `main`, etc.).
3. Evaluates PR risk score, blast radius on sensitive files, and missing test deltas against thresholds configured in `.gh-stats.yml` or `--fail-on`.
4. **Blocks the push** with actionable remediation guidance if the threshold is met or exceeded:

```text
🔍 gh-stats: Running shift-left pre-push risk evaluation against [origin/main]...
📊 Risk Rating: HIGH (Score: 70/100) | Files: 8 | +380 / -45
🚨 Sensitive Files (2):
   • .github/workflows/deploy.yml (CI/CD Pipelines)
   • migrations/004_accounts.sql (Database Migrations)

❌ PUSH BLOCKED: Risk level [HIGH] meets or exceeds threshold [HIGH] (Score: 70/100)
💡 SRE Shift-Left Guidance:
   • Add automated tests for the +380 lines of newly added code.
   • Sensitive infrastructure, security, or database migration files were modified.
💡 To bypass this check:
   git push --no-verify
   OR: SKIP_GH_STATS=1 git push
```

### Bypassing Hooks
When rapid or emergency pushes are necessary, developers can bypass the hook using standard git flags or an environment variable:
```bash
git push --no-verify
# OR
SKIP_GH_STATS=1 git push
```

### Standalone Examples
Reference standalone hook scripts are available in the [`examples/hooks`](examples/hooks) directory:
- [`examples/hooks/pre-push`](examples/hooks/pre-push): Pre-push hook script.
- [`examples/hooks/pre-commit`](examples/hooks/pre-commit): Pre-commit hook script for staged files.

---

## 🧪 Testing

Run unit tests and race condition checks:

```bash
go test -race -v ./...
go vet ./...
```

---

## 🔒 Security & SRE Best Practices

1. **Least Privilege Permissions:** The action only requires `contents: read` to examine code and `security-events: write` to post SARIF alerts to GitHub Code Scanning.
2. **Deterministic & Fast:** Avoids network calls during diff/repo analysis, running git queries locally with timeouts.
3. **No Code Uploaded to External Services:** All processing happens entirely within the GitHub Actions runner.
