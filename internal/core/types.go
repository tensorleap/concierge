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
	ID                  string            `json:"id"`
	CapturedAt          time.Time         `json:"capturedAt"`
	WorktreeFingerprint string            `json:"worktreeFingerprint"`
	SelectedModelPath   string            `json:"selectedModelPath,omitempty"`
	Repository          RepositoryState   `json:"repository"`
	FileHashes          map[string]string `json:"fileHashes,omitempty"`
	Runtime             RuntimeState      `json:"runtime,omitempty"`
	LeapCLI             LeapCLIState      `json:"leapCli,omitempty"`
}

// RuntimeState captures lightweight runtime/tooling fingerprints.
type RuntimeState struct {
	ProbeRan          bool     `json:"probeRan"`
	PythonFound       bool     `json:"pythonFound"`
	PythonExecutable  string   `json:"pythonExecutable,omitempty"`
	PythonVersion     string   `json:"pythonVersion,omitempty"`
	RequirementsFiles []string `json:"requirementsFiles,omitempty"`
}

// LeapCLIState captures non-destructive CLI/auth/server readiness probes.
type LeapCLIState struct {
	ProbeRan            bool   `json:"probeRan"`
	Available           bool   `json:"available"`
	Version             string `json:"version,omitempty"`
	Authenticated       bool   `json:"authenticated"`
	ServerInfoReachable bool   `json:"serverInfoReachable"`
	ServerInfoError     string `json:"serverInfoError,omitempty"`
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

// ModelCandidate captures one discovered model path candidate and its origin.
type ModelCandidate struct {
	Path   string `json:"path"`
	Source string `json:"source,omitempty"`
}

// IntegrationContracts captures discovered interface symbols from the integration entry file.
type IntegrationContracts struct {
	EntryFile                string           `json:"entryFile"`
	LoadModelFunctions       []string         `json:"loadModelFunctions,omitempty"`
	PreprocessFunctions      []string         `json:"preprocessFunctions,omitempty"`
	InputEncoders            []string         `json:"inputEncoders,omitempty"`
	GroundTruthEncoders      []string         `json:"groundTruthEncoders,omitempty"`
	IntegrationTestFunctions []string         `json:"integrationTestFunctions,omitempty"`
	IntegrationTestCalls     []string         `json:"integrationTestCalls,omitempty"`
	ModelCandidates          []ModelCandidate `json:"modelCandidates,omitempty"`
	ResolvedModelPath        string           `json:"resolvedModelPath,omitempty"`
}

// IntegrationStatus summarizes what is currently missing or invalid.
type IntegrationStatus struct {
	Missing   []string              `json:"missing"`
	Issues    []Issue               `json:"issues"`
	Contracts *IntegrationContracts `json:"contracts,omitempty"`
}

// Ready reports whether integration has no known blockers.
func (s IntegrationStatus) Ready() bool {
	return len(s.Missing) == 0 && len(s.Issues) == 0
}

// AgentRepoContext captures deterministic, step-scoped repository facts for agent tasks.
type AgentRepoContext struct {
	RepoRoot             string   `json:"repoRoot"`
	EntryFile            string   `json:"entryFile,omitempty"`
	BinderFile           string   `json:"binderFile,omitempty"`
	LeapYAMLBoundary     string   `json:"leapYamlBoundary,omitempty"`
	SelectedModelPath    string   `json:"selectedModelPath,omitempty"`
	ModelCandidates      []string `json:"modelCandidates,omitempty"`
	DecoratorInventory   []string `json:"decoratorInventory,omitempty"`
	IntegrationTestCalls []string `json:"integrationTestCalls,omitempty"`
	BlockingIssues       []string `json:"blockingIssues,omitempty"`
	ValidationFindings   []string `json:"validationFindings,omitempty"`
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

// CommitMetadata describes an audited git commit created for one iteration.
type CommitMetadata struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

// GitDecision captures change-control outcomes for one execution result.
type GitDecision struct {
	FinalResult ExecutionResult `json:"finalResult"`
	Commit      *CommitMetadata `json:"commit,omitempty"`
	Notes       []string        `json:"notes,omitempty"`
	Evidence    []EvidenceItem  `json:"evidence,omitempty"`
}

// ValidationResult describes post-execution acceptance checks.
type ValidationResult struct {
	Passed bool    `json:"passed"`
	Issues []Issue `json:"issues,omitempty"`
}

// CheckStatus is a user-facing verification state for one checked requirement.
type CheckStatus string

const (
	CheckStatusPass    CheckStatus = "pass"
	CheckStatusWarning CheckStatus = "warning"
	CheckStatusFail    CheckStatus = "fail"
)

// VerifiedCheck captures one explicitly verified requirement row for UI/report output.
type VerifiedCheck struct {
	StepID   EnsureStepID `json:"stepId"`
	Label    string       `json:"label"`
	Status   CheckStatus  `json:"status"`
	Blocking bool         `json:"blocking,omitempty"`
	Issues   []Issue      `json:"issues,omitempty"`
}

// IterationReport is the final stage payload for one orchestration loop.
type IterationReport struct {
	GeneratedAt time.Time        `json:"generatedAt"`
	SnapshotID  string           `json:"snapshotId"`
	Step        EnsureStep       `json:"step"`
	Applied     bool             `json:"applied"`
	Evidence    []EvidenceItem   `json:"evidence,omitempty"`
	Checks      []VerifiedCheck  `json:"checks,omitempty"`
	Validation  ValidationResult `json:"validation"`
	Commit      *CommitMetadata  `json:"commit,omitempty"`
	Notes       []string         `json:"notes,omitempty"`
}
