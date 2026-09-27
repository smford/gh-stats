package sarif

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	CurrentSchema  = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	CurrentVersion = "2.1.0"
	ToolName       = "gh-stats"
	ToolVersion    = "1.0.0"
	ToolURI        = "https://github.com/smford/gh-stats"
)

// Builder helps construct a compliant SARIF report.
type Builder struct {
	toolName    string
	toolVersion string
	toolURI     string
	rules       []Rule
	ruleIndices map[string]int
	results     []Result
}

// NewBuilder initializes a new SARIF report builder.
func NewBuilder() *Builder {
	return &Builder{
		toolName:    ToolName,
		toolVersion: ToolVersion,
		toolURI:     ToolURI,
		rules:       make([]Rule, 0),
		ruleIndices: make(map[string]int),
		results:     make([]Result, 0),
	}
}

// AddRule registers a rule/descriptor if not already present, returning its index.
func (b *Builder) AddRule(rule Rule) int {
	if idx, exists := b.ruleIndices[rule.ID]; exists {
		return idx
	}
	idx := len(b.rules)
	b.ruleIndices[rule.ID] = idx
	b.rules = append(b.rules, rule)
	return idx
}

// AddResult adds a finding/result to the report, automatically associating with the rule.
func (b *Builder) AddResult(ruleID string, level string, text string, markdown string, fileURI string, line int, properties map[string]any) {
	idx, exists := b.ruleIndices[ruleID]
	if !exists {
		// Auto-register a default rule placeholder if rule wasn't explicitly added
		idx = b.AddRule(Rule{
			ID:               ruleID,
			Name:             ruleID,
			ShortDescription: MultiformatMessage{Text: ruleID},
			DefaultConfiguration: &RuleConfiguration{
				Level: level,
			},
		})
	}

	result := Result{
		RuleID:    ruleID,
		RuleIndex: idx,
		Level:     level,
		Message: MultiformatMessage{
			Text:     text,
			Markdown: markdown,
		},
		Properties: properties,
	}

	if fileURI != "" {
		loc := Location{
			PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{
					URI:       fileURI,
					URIBaseID: "%SRCROOT%",
				},
			},
		}
		if line > 0 {
			loc.PhysicalLocation.Region = &Region{
				StartLine:   line,
				StartColumn: 1,
			}
		}
		result.Locations = []Location{loc}
	}

	b.results = append(b.results, result)
}

// Build generates the complete Report object.
func (b *Builder) Build() Report {
	return Report{
		Schema:  CurrentSchema,
		Version: CurrentVersion,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:           b.toolName,
						Version:        b.toolVersion,
						InformationURI: b.toolURI,
						Rules:          b.rules,
					},
				},
				Results: b.results,
			},
		},
	}
}

// WriteTo writes formatted SARIF JSON to a writer, implementing io.WriterTo.
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	report := b.Build()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return 0, err
	}
	data = append(data, '\n')
	n, err := w.Write(data)
	return int64(n), err
}

// WriteFile writes the SARIF report to a file on disk.
func (b *Builder) WriteFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create SARIF file %s: %w", path, err)
	}
	defer f.Close()

	if _, err := b.WriteTo(f); err != nil {
		return fmt.Errorf("failed to write SARIF data: %w", err)
	}
	return nil
}
