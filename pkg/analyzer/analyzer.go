package analyzer

import (
	"path/filepath"
	"strings"

	"github.com/smford/gh-stats/pkg/config"
	"github.com/smford/gh-stats/pkg/sarif"
)

// SensitivePattern defines criteria for identifying high-blast-radius files.
type SensitivePattern struct {
	Category    string
	Description string
	Match       func(path string) bool
}

// SensitivePatternsWithConfig returns default sensitive patterns merged with user-defined custom patterns.
func SensitivePatternsWithConfig(cfg *config.Config) []SensitivePattern {
	patterns := DefaultSensitivePatterns()
	if cfg == nil {
		return patterns
	}

	for _, cp := range cfg.BlastRadius.CustomPatterns {
		patternStr := cp.Pattern
		cat := cp.Category
		desc := cp.Description
		patterns = append(patterns, SensitivePattern{
			Category:    cat,
			Description: desc,
			Match: func(path string) bool {
				return config.MatchGlob(patternStr, path)
			},
		})
	}
	return patterns
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
				return strings.HasSuffix(p, ".sql") ||
					strings.Contains(p, "migration") ||
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

// IsGeneratedFile checks if a file is machine-generated or minified.
func IsGeneratedFile(path string) bool {
	p := filepath.ToSlash(strings.ToLower(path))
	base := filepath.Base(p)
	return strings.HasSuffix(p, ".pb.go") ||
		strings.HasSuffix(p, "_gen.go") ||
		strings.HasSuffix(p, ".generated.go") ||
		strings.HasSuffix(p, ".generated.ts") ||
		strings.HasSuffix(p, ".generated.js") ||
		strings.HasSuffix(p, ".min.js") ||
		strings.HasSuffix(p, ".min.css") ||
		strings.HasSuffix(p, "bundle.js") ||
		strings.HasSuffix(p, ".swagger.json") ||
		strings.HasPrefix(base, "mock_") ||
		strings.HasPrefix(p, "vendor/") ||
		strings.Contains(p, "/vendor/") ||
		strings.HasPrefix(p, "third_party/") ||
		strings.Contains(p, "/third_party/") ||
		strings.HasPrefix(p, "dist/") ||
		strings.Contains(p, "/dist/") ||
		strings.HasPrefix(p, "build/") ||
		strings.Contains(p, "/build/")
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

	RulePRReviewers = sarif.Rule{
		ID:   "GHSTATS007-PR-REVIEWERS",
		Name: "RecommendedDomainReviewers",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Domain expert reviewers recommended based on historical commit patterns",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Routing PRs to engineers with demonstrated context on the modified components reduces production outages and speeds up code reviews.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RulePRCILatency = sarif.Rule{
		ID:   "GHSTATS008-PR-CI-LATENCY",
		Name: "CIPipelineLatencyBottleneck",
		ShortDescription: sarif.MultiformatMessage{
			Text: "CI pipeline check run exceeds acceptable latency threshold",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Long-running CI jobs inflate developer turnaround times, elevate Mean Time to Detect (MTTD), and delay deployment rollouts.",
		},
		Help: &sarif.MultiformatMessage{
			Text:     "Optimize slow test suites, parallelize matrix jobs, or introduce test caching to reduce CI pipeline latency.",
			Markdown: "### SRE CI Guidance\n- **Target Turnaround (<15m)**: Keep pull request CI runs snappy to prevent context switching.\n- **Bottleneck Remediation**: Parallelize test shards, split integration tests, or leverage build caches.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RulePRCIFlakiness = sarif.Rule{
		ID:   "GHSTATS009-PR-CI-FLAKINESS",
		Name: "CIPipelineFlakinessDetected",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Flaky CI check run exhibited conflicting results on the same commit",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "A check run failed on an initial run but passed on retry without any code changes on the same commit SHA, indicating test flakiness or environmental instability.",
		},
		Help: &sarif.MultiformatMessage{
			Text:     "Quarantine flaky tests and investigate race conditions, timing issues, or external dependencies.",
			Markdown: "### SRE Flakiness Guidance\n- **Erosion of Trust**: Flaky tests train engineers to ignore CI signals and blindly re-run jobs.\n- **Action**: Quarantine the flaky test immediately and run in isolation to reproduce timing or concurrency bugs.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RuleReleaseSummary = sarif.Rule{
		ID:   "GHSTATS201-RELEASE-SUMMARY",
		Name: "ReleaseComparisonSummary",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Release comparison delta and SRE deployment readiness summary",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Aggregates commit velocity, test-to-code ratio, breaking changes, and blast radius risk scoring across release milestones.",
		},
		Help: &sarif.MultiformatMessage{
			Text:     "Verify that breaking changes are documented in migration guides, blast radius files have appropriate sign-offs, and adequate test coverage delta is present.",
			Markdown: "### Release Readiness Guidance\n- **Verification**: Review breaking changes and ensure API migration guides are published.\n- **Blast Radius**: Verify infrastructure, pipeline, and schema changes before deployment.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "note"},
	}

	RuleReleaseBreaking = sarif.Rule{
		ID:   "GHSTATS202-RELEASE-BREAKING",
		Name: "BreakingChangeDetectedInRelease",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Breaking changes or schema migrations detected in release comparison",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Commits with breaking change syntax or modifications to database schema migrations alter backwards compatibility and require deployment precautions.",
		},
		Help: &sarif.MultiformatMessage{
			Text:     "Ensure backwards compatibility, data migration scripts, and roll-forward/roll-back verification are in place.",
			Markdown: "### Breaking Change SRE Guidance\n- **Zero-Downtime Deployments**: Use expand-contract pattern for schema migrations.\n- **Deprecations**: Ensure downstream consumers received deprecation notices.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}

	RuleReleaseBlastRadius = sarif.Rule{
		ID:   "GHSTATS203-RELEASE-BLAST-RADIUS",
		Name: "ReleaseHighBlastRadiusDetected",
		ShortDescription: sarif.MultiformatMessage{
			Text: "Release comparison delta modifies critical infrastructure, pipeline, or dependency files",
		},
		FullDescription: &sarif.MultiformatMessage{
			Text: "Changes to CI/CD workflows, infrastructure manifests, database migrations, or core dependencies across release milestones elevate deployment risk.",
		},
		Help: &sarif.MultiformatMessage{
			Text:     "Review sensitive file changes to ensure cloud configurations, pipeline security, and infrastructure changes are validated prior to production release.",
			Markdown: "### Blast Radius Verification\n- **Sign-off**: Ensure infrastructure and security changes have explicit platform engineer sign-off.",
		},
		DefaultConfiguration: &sarif.RuleConfiguration{Level: "warning"},
	}
)

