package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/smford/gh-stats/pkg/sarif"
)

// SensitivePattern defines criteria for identifying high-blast-radius files.
type SensitivePattern struct {
	Category    string
	Description string
	Match       func(path string) bool
}

// DefaultSensitivePatterns returns standard SRE sensitive file classifications.
func DefaultSensitivePatterns() []SensitivePattern {
	return []SensitivePattern{
		{
			Category:    "CI/CD Pipelines",
			Description: "Modifications to deployment workflows, GitHub Actions, or build steps can alter delivery pipelines.",
			Match: func(path string) bool {
				p := filepath.ToSlash(path)
				return strings.HasPrefix(p, ".github/workflows/") ||
					strings.HasPrefix(p, ".gitlab-ci") ||
					strings.HasPrefix(p, ".circleci/") ||
					strings.HasSuffix(p, "Jenkinsfile")
			},
		},
		{
			Category:    "Infrastructure & Containers",
			Description: "Changes to Kubernetes manifests, Terraform files, or Docker configurations alter runtime topologies.",
			Match: func(path string) bool {
				p := filepath.ToSlash(strings.ToLower(path))
				return strings.HasSuffix(p, ".tf") ||
					strings.HasSuffix(p, ".tfvars") ||
					strings.Contains(p, "k8s/") ||
					strings.Contains(p, "helm/") ||
					strings.Contains(p, "charts/") ||
					strings.Contains(p, "dockerfile") ||
					strings.Contains(p, "compose.yml") ||
					strings.Contains(p, "compose.yaml")
			},
		},
		{
			Category:    "Database Migrations",
			Description: "Database schema migrations can lock tables, cause replication lag, or induce breaking schema changes.",
			Match: func(path string) bool {
				p := filepath.ToSlash(strings.ToLower(path))
				return strings.Contains(p, "migration") ||
					strings.Contains(p, "/migrations/") ||
					(strings.Contains(p, "db/") && strings.HasSuffix(p, ".sql"))
			},
		},
		{
			Category:    "Dependencies & Core Config",
			Description: "Updating lockfiles or dependency manifests introduces third-party code into production.",
			Match: func(path string) bool {
				base := filepath.Base(path)
				return base == "go.mod" || base == "go.sum" ||
					base == "package.json" || base == "package-lock.json" || base == "pnpm-lock.yaml" ||
					base == "yarn.lock" || base == "Cargo.toml" || base == "Cargo.lock" ||
					base == "pom.xml" || base == "requirements.txt" || base == "Pipfile.lock"
			},
		},
		{
			Category:    "Auth & Security",
			Description: "Changes in authentication or authorization logic can introduce security vulnerabilities.",
			Match: func(path string) bool {
				p := filepath.ToSlash(strings.ToLower(path))
				return strings.Contains(p, "/auth/") ||
					strings.Contains(p, "/security/") ||
					strings.Contains(p, "/jwt/") ||
					strings.Contains(p, "/oauth/")
			},
		},
	}
}

// IsTestFile checks if a file path belongs to automated tests.
func IsTestFile(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.HasSuffix(p, "_test.go") ||
		strings.HasSuffix(p, ".test.js") ||
		strings.HasSuffix(p, ".test.ts") ||
		strings.HasSuffix(p, ".test.jsx") ||
		strings.HasSuffix(p, ".test.tsx") ||
		strings.HasSuffix(p, ".spec.js") ||
		strings.HasSuffix(p, ".spec.ts") ||
		strings.HasSuffix(p, ".spec.jsx") ||
		strings.HasSuffix(p, ".spec.tsx") ||
		strings.HasSuffix(p, "_test.py") ||
		strings.HasPrefix(filepath.Base(p), "test_") ||
		strings.HasPrefix(p, "tests/") ||
		strings.Contains(p, "/tests/") ||
		strings.HasPrefix(p, "test/") ||
		strings.Contains(p, "/test/") ||
		strings.HasPrefix(p, "spec/") ||
		strings.Contains(p, "/spec/")
}

// IsDocumentationFile checks if a file is documentation.
func IsDocumentationFile(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	return strings.HasSuffix(p, ".md") ||
		strings.HasSuffix(p, ".rst") ||
		strings.HasSuffix(p, ".txt") ||
		strings.HasPrefix(p, "docs/")
}

// Rules definitions for SARIF reporting
var (
	RulePRSummary = sarif.Rule{
		ID:   "GHSTATS001-PR-SUMMARY",
		Name: "PullRequestSREHealthSummary",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Pull request size, blast radius, and SRE reliability health summary",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Aggregates lines changed, test-to-code ratio, and blast radius risk scoring for code reviewers and release engineers.",
		},
		Help: &sarif.MultiformatMessage{
			Text: "Keep PRs small (<400 lines) and ensure test coverage to reduce deployment risk and MTTR.",
			Markdown: "### SRE Guidance\n- **Small PRs (<400 lines)**: Merge 3x faster with 70% fewer production defects.\n- **Blast Radius**: Watch out for infrastructure and CI/CD changes.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RulePRSize = sarif.Rule{
		ID:   "GHSTATS002-PR-SIZE",
		Name: "PullRequestExcessiveSize",
		ShortDescription: sarif.MultiformatMessage{
			Text: "PR exceeds recommended size limits, increasing review fatigue and outage risk",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Large PRs correlate strongly with higher defect escape rates, production outages, and delayed review cycles.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RulePRTestRatio = sarif.Rule{
		ID:   "GHSTATS003-PR-TEST-RATIO",
		Name: "MissingTestCoverageDelta",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Significant production code changed without corresponding test additions",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Modifying production logic without automated test updates undermines test suite efficacy and increases regression probability.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RulePRBlastRadius = sarif.Rule{
		ID:   "GHSTATS004-PR-BLAST-RADIUS",
		Name: "HighBlastRadiusDetected",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Pull request modifies critical infrastructure, pipeline, or dependency files",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Modifications to CI/CD workflows, infrastructure manifests, database migrations, or core dependencies require extra review diligence.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RuleRepoSummary = sarif.Rule{
		ID:   "GHSTATS101-REPO-SUMMARY",
		Name: "RepositorySREHealthSummary",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Repository architectural metrics, commit velocity, and test density summary",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Holistic overview of codebase volume, language breakdown, test density, and contributor distribution.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RuleRepoHotspots = sarif.Rule{
		ID:   "GHSTATS102-REPO-HOTSPOTS",
		Name: "HighChurnHotspotDetected",
		ShortDescription: sarif.MultiformatMessage{
			Text: "File has experienced unusually high change frequency (churn hotspot)",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Code hotspots represent files modified disproportionately often, representing high cognitive load and frequent regressions.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RuleRepoBusFactor = sarif.Rule{
		ID:   "GHSTATS103-REPO-BUS-FACTOR",
		Name: "HighContributorConcentration",
		ShortDescription: sarif.MultiformatMessage{
			Text: "High contributor concentration detected (bus factor risk)",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "When the vast majority of commits originate from a single contributor, knowledge silo risk threatens project continuity.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RulePRStale = sarif.Rule{
		ID:   "GHSTATS005-PR-STALE",
		Name: "StalePullRequestRisk",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Pull request has remained open for an extended period, increasing merge conflict risk",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Long-lived branches drift away from main, creating painful merge conflicts, latent integration defects, and prolonged lead time.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RulePRDiscussionChurn = sarif.Rule{
		ID:   "GHSTATS006-PR-DISCUSSION-CHURN",
		Name: "HighDiscussionFriction",
		ShortDescription: sarif.MultiformatMessage{
			Text: "High comment volume detected indicating design misalignment or review friction",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "PRs with excessive comment counts often signal unclear specifications or contentious architectural debates that benefit from real-time sync.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RuleRepoAPIMetadata = sarif.Rule{
		ID:   "GHSTATS104-REPO-API-METADATA",
		Name: "GitHubRepositoryHealth",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Repository GitHub ecosystem metrics (stars, issue backlog, activity)",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Ecosystem overview retrieved via GitHub API reflecting project adoption and issue queue health.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}
)
