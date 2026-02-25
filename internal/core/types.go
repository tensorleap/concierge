package core

import "time"

// Stage is a deterministic orchestration stage name.
type Stage string

const (
	StageSnapshot Stage = "snapshot"
	StageInspect  Stage = "inspect"
	StagePlan     Stage = "plan"
	StageExecute  Stage = "execute"
	StageValidate Stage = "validate"
	StageReport   Stage = "report"
)

var defaultStages = []Stage{
	StageSnapshot,
	StageInspect,
	StagePlan,
	StageExecute,
	StageValidate,
	StageReport,
}

// DefaultStages returns the canonical orchestration stage ordering.
func DefaultStages() []Stage {
	stages := make([]Stage, len(defaultStages))
	copy(stages, defaultStages)
	return stages
}

// SnapshotRequest describes what workspace should be captured.
type SnapshotRequest struct {
	RepoRoot string `json:"repoRoot"`
}

// RepositoryState captures git-aware workspace identity at snapshot time.
type RepositoryState struct {
	Root    string `json:"root"`
	GitRoot string `json:"gitRoot"`
	Branch  string `json:"branch"`
	Head    string `json:"head"`
	Dirty   bool   `json:"dirty"`
}

// WorkspaceSnapshot is an immutable iteration input.
type WorkspaceSnapshot struct {
	ID                  string          `json:"id"`
	CapturedAt          time.Time       `json:"capturedAt"`
	WorktreeFingerprint string          `json:"worktreeFingerprint"`
	Repository          RepositoryState `json:"repository"`
}

// Severity represents the importance of an inspection or validation issue.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Issue is a structured finding discovered during inspect or validate phases.
type Issue struct {
	Code     IssueCode      `json:"code"`
	Message  string         `json:"message"`
	Severity Severity       `json:"severity"`
	Scope    IssueScope     `json:"scope,omitempty"`
	Location *IssueLocation `json:"location,omitempty"`
}

// IssueLocation points to a concrete source location when an issue is file/symbol based.
// Some issue kinds are global contract problems and intentionally omit location.
type IssueLocation struct {
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

// IntegrationStatus summarizes what is currently missing or invalid.
type IntegrationStatus struct {
	Missing []string `json:"missing"`
	Issues  []Issue  `json:"issues"`
}

// Ready reports whether integration has no known blockers.
func (s IntegrationStatus) Ready() bool {
	return len(s.Missing) == 0 && len(s.Issues) == 0
}

// EnsureStep is one deterministic action the engine can apply.
type EnsureStep struct {
	ID          EnsureStepID `json:"id"`
	Description string       `json:"description"`
}

// ExecutionPlan captures the next step and optional backlog.
type ExecutionPlan struct {
	Primary    EnsureStep   `json:"primary"`
	Additional []EnsureStep `json:"additional,omitempty"`
}

// EvidenceItem is a minimal proof artifact emitted by execution.
type EvidenceItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ExecutionResult describes the output of running a single ensure-step.
type ExecutionResult struct {
	Step     EnsureStep     `json:"step"`
	Applied  bool           `json:"applied"`
	Summary  string         `json:"summary"`
	Evidence []EvidenceItem `json:"evidence,omitempty"`
}

// ValidationResult describes post-execution acceptance checks.
type ValidationResult struct {
	Passed bool    `json:"passed"`
	Issues []Issue `json:"issues,omitempty"`
}

// IterationReport is the final stage payload for one orchestration loop.
type IterationReport struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	SnapshotID  string           `json:"snapshotId"`
	Step        EnsureStep       `json:"step"`
	Validation  ValidationResult `json:"validation"`
	Notes       []string         `json:"notes,omitempty"`
}
