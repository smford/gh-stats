package sarif

// Report represents a complete SARIF v2.1.0 report.
type Report struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run represents an individual run of an analysis tool.
type Run struct {
	Tool    Tool     `json:"tool"`
	Results []Result `json:"results"`
}

// Tool describes the analysis tool that was run.
type Tool struct {
	Driver Driver `json:"driver"`
}

// Driver provides details about the tool driver.
type Driver struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	InformationURI string `json:"informationUri,omitempty"`
	Rules          []Rule `json:"rules"`
}

// Rule defines a metadata descriptor for a rule/check.
type Rule struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name,omitempty"`
	ShortDescription     MultiformatMessage   `json:"shortDescription"`
	FullDescription      *MultiformatMessage  `json:"fullDescription,omitempty"`
	Help                 *MultiformatMessage  `json:"help,omitempty"`
	DefaultConfiguration *RuleConfiguration   `json:"defaultConfiguration,omitempty"`
	Properties           map[string]any       `json:"properties,omitempty"`
}

// RuleConfiguration specifies the default level and settings for a rule.
type RuleConfiguration struct {
	Level string `json:"level"` // "none", "note", "warning", "error"
}

// Result describes a single analysis finding, metric, or event.
type Result struct {
	RuleID    string           `json:"ruleId"`
	RuleIndex int              `json:"ruleIndex"`
	Level     string           `json:"level"` // "none", "note", "warning", "error"
	Message   MultiformatMessage `json:"message"`
	Locations []Location       `json:"locations,omitempty"`
	Properties map[string]any  `json:"properties,omitempty"`
}

// Location specifies the target file and line.
type Location struct {
	PhysicalLocation PhysicalLocation `json:"physicalLocation"`
}

// PhysicalLocation holds the file URI and region.
type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           *Region          `json:"region,omitempty"`
}

// ArtifactLocation holds the path to the file.
type ArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// Region identifies a portion of the artifact.
type Region struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// MultiformatMessage supports both plain text and markdown.
type MultiformatMessage struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}
