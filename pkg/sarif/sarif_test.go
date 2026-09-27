package sarif

import (
	"bytes"
	"encoding/json"
	"testing"
)


func TestSARIFBuildAndSerialize(t *testing.T) {
	b := NewBuilder()

	rule := Rule{
		ID:   "TEST001",
		Name: "TestRule",
		ShortDescription: MultiformatMessage{
			Text: "A test rule description",
		},
		DefaultConfiguration: &RuleConfiguration{
			Level: "warning",
		},
	}
	idx := b.AddRule(rule)
	if idx != 0 {
		t.Fatalf("expected rule index 0, got %d", idx)
	}

	// Add same rule again, should return existing index
	idx2 := b.AddRule(rule)
	if idx2 != 0 {
		t.Fatalf("expected rule index 0 on duplicate add, got %d", idx2)
	}

	b.AddResult(
		"TEST001",
		"warning",
		"Result message text",
		"### Markdown result",
		"main.go",
		42,
		map[string]any{"metric": 100},
	)

	var buf bytes.Buffer
	if _, err := b.WriteTo(&buf); err != nil {
		t.Fatalf("failed to write SARIF: %v", err)
	}

	var parsed Report
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse generated SARIF JSON: %v", err)
	}

	if parsed.Version != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %s", parsed.Version)
	}
	if len(parsed.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(parsed.Runs))
	}
	run := parsed.Runs[0]
	if run.Tool.Driver.Name != ToolName {
		t.Errorf("expected driver name %s, got %s", ToolName, run.Tool.Driver.Name)
	}
	if len(run.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(run.Results))
	}
	res := run.Results[0]
	if res.RuleID != "TEST001" {
		t.Errorf("expected ruleId TEST001, got %s", res.RuleID)
	}
	if res.Level != "warning" {
		t.Errorf("expected level warning, got %s", res.Level)
	}
	if len(res.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(res.Locations))
	}
	loc := res.Locations[0]
	if loc.PhysicalLocation.ArtifactLocation.URI != "main.go" {
		t.Errorf("expected uri main.go, got %s", loc.PhysicalLocation.ArtifactLocation.URI)
	}
	if loc.PhysicalLocation.Region.StartLine != 42 {
		t.Errorf("expected startLine 42, got %d", loc.PhysicalLocation.Region.StartLine)
	}
}
