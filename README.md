# ⚡ gh-stats

A production-grade GitHub Action and CLI tool written in Go that generates deep **SRE & developer reliability statistics** for Pull Requests or entire repositories, outputting standards-compliant **SARIF v2.1.0** reports directly into GitHub Code Scanning.

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
| `target` | Analysis mode: `'pr'`, `'repo'`, or `'auto'` (auto-detects based on event) | `'auto'` | No |
| `base-ref` | Base git reference for PR diff | `github.base_ref` | No |
| `head-ref` | Head git reference for PR diff | `HEAD` | No |
| `output` | Destination file path for generated SARIF report | `gh-stats.sarif` | No |
| `commit-limit` | Maximum commit history to analyze in `repo` mode | `200` | No |
| `upload-sarif` | Automatically upload SARIF to GitHub Code Scanning via `@actions/upload-sarif` | `'true'` | No |
| `category` | SARIF category label in GitHub Code Scanning | `gh-stats` | No |
| `comment-pr` | Post or update a live sticky Markdown summary comment on the PR conversation thread | `'false'` | No |
| `fail-on` | Enforce risk budget gating: fail job if risk meets/exceeds threshold (`'CRITICAL'`, `'HIGH'`, `'MEDIUM'`, `'LOW'`) | `''` (disabled) | No |
| `config-path` | Path to `.gh-stats.yml` configuration file (auto-discovers `.gh-stats.yml` or `.github/.gh-stats.yml` if omitted) | `''` | No |
| `token` | GitHub token for authentication (API stats, sticky comment, SARIF upload) | `${{ github.token }}` | No |

### Outputs

| Output | Description |
| :--- | :--- |
| `sarif-file` | Path to the generated SARIF report file |
| `target` | Resolved analysis target (`pr` or `repo`) |

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
| `GHSTATS101-REPO-SUMMARY` | `note` | Repo | Architecture overview, test density %, language breakdown |
| `GHSTATS102-REPO-HOTSPOTS` | `note` | Repo | High-churn files identified across commit history |
| `GHSTATS103-REPO-BUS-FACTOR` | `warning` | Repo | Single contributor accounts for >75% of commits |
| `GHSTATS104-REPO-API-METADATA` | `note` | Repo | GitHub ecosystem metrics (stars, forks, open issue queue) |

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

# Use a custom configuration file
./gh-stats -target=pr -config=.github/.gh-stats.yml
```

### CLI Flags:
```text
  -target string
        Target scope: 'pr', 'repo', or 'auto' (default "auto")
  -base string
        Base ref for PR comparison (e.g. origin/main)
  -head string
        Head ref for PR comparison (default "HEAD")
  -config string
        Path to .gh-stats.yml configuration file
  -output string
        Path to output SARIF file (default "gh-stats.sarif")
  -repo-path string
        Path to git repository (default ".")
  -commit-limit int
        Maximum commit history to examine for repo hotspots (default 200)
  -fail-on string
        Fail workflow if PR risk meets/exceeds threshold (e.g. 'HIGH', 'CRITICAL')
  -comment-pr
        Post or update a sticky summary comment on the PR
  -quiet
        Suppress stdout output
```

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