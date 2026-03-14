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
	ID                      string                  `json:"id"`
	CapturedAt              time.Time               `json:"capturedAt"`
	WorktreeFingerprint     string                  `json:"worktreeFingerprint"`
	SelectedModelPath       string                  `json:"selectedModelPath,omitempty"`
	ConfirmedEncoderMapping *EncoderMappingContract `json:"confirmedEncoderMapping,omitempty"`
	Repository              RepositoryState         `json:"repository"`
	FileHashes              map[string]string       `json:"fileHashes,omitempty"`
	Runtime                 RuntimeState            `json:"runtime,omitempty"`
	RuntimeProfile          *LocalRuntimeProfile    `json:"runtimeProfile,omitempty"`
	LeapCLI                 LeapCLIState            `json:"leapCli,omitempty"`
}

// RuntimeState captures lightweight runtime/tooling fingerprints.
type RuntimeState struct {
	ProbeRan              bool     `json:"probeRan"`
	PoetryFound           bool     `json:"poetryFound"`
	PoetryExecutable      string   `json:"poetryExecutable,omitempty"`
	PoetryVersion         string   `json:"poetryVersion,omitempty"`
	SupportedProject      bool     `json:"supportedProject"`
	ProjectSupportReason  string   `json:"projectSupportReason,omitempty"`
	PyProjectPresent      bool     `json:"pyprojectPresent"`
	PoetryLockPresent     bool     `json:"poetryLockPresent"`
	AmbientVirtualEnv     string   `json:"ambientVirtualEnv,omitempty"`
	AmbientCondaPrefix    string   `json:"ambientCondaPrefix,omitempty"`
	RequirementsFiles     []string `json:"requirementsFiles,omitempty"`
	ResolvedInterpreter   string   `json:"resolvedInterpreter,omitempty"`
	ResolvedPythonVersion string   `json:"resolvedPythonVersion,omitempty"`
}

// LocalRuntimeProfile captures the persisted Poetry runtime selected for local execution.
type LocalRuntimeProfile struct {
	Kind              string                    `json:"kind"`
	PoetryExecutable  string                    `json:"poetryExecutable,omitempty"`
	PoetryVersion     string                    `json:"poetryVersion,omitempty"`
	InterpreterPath   string                    `json:"interpreterPath,omitempty"`
	PythonVersion     string                    `json:"pythonVersion,omitempty"`
	ConfirmationMode  string                    `json:"confirmationMode,omitempty"`
	DependenciesReady bool                      `json:"dependenciesReady,omitempty"`
	CodeLoaderReady   bool                      `json:"codeLoaderReady,omitempty"`
	CodeLoaderDeclaredInProject bool            `json:"codeLoaderDeclaredInProject,omitempty"`
	CodeLoader        CodeLoaderCapabilityState `json:"codeLoader,omitempty"`
	Fingerprint       RuntimeProfileFingerprint `json:"fingerprint"`
}

// CodeLoaderCapabilityState captures the installed code-loader version and the
// validator surfaces it exposes in the resolved Poetry environment.
type CodeLoaderCapabilityState struct {
	ProbeSucceeded                bool   `json:"probeSucceeded,omitempty"`
	Version                       string `json:"version,omitempty"`
	SupportsGuideLocalStatusTable bool   `json:"supportsGuideLocalStatusTable,omitempty"`
	SupportsCheckDataset          bool   `json:"supportsCheckDataset,omitempty"`
}

// RuntimeProfileFingerprint captures the inputs that invalidate a persisted runtime profile.
type RuntimeProfileFingerprint struct {
	ProjectRoot     string `json:"projectRoot,omitempty"`
	PyProjectHash   string `json:"pyprojectHash,omitempty"`
	PoetryLockHash  string `json:"poetryLockHash,omitempty"`
	InterpreterPath string `json:"interpreterPath,omitempty"`
	PythonVersion   string `json:"pythonVersion,omitempty"`
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

// InputGTEvidence is one evidence snippet supporting a discovered input/ground-truth candidate.
type InputGTEvidence struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet,omitempty"`
}

// InputGTCandidate is one discovered input/ground-truth candidate.
type InputGTCandidate struct {
	Name         string            `json:"name"`
	SemanticHint string            `json:"semanticHint,omitempty"`
	ShapeHint    string            `json:"shapeHint,omitempty"`
	DTypeHint    string            `json:"dtypeHint,omitempty"`
	Confidence   string            `json:"confidence,omitempty"`
	Evidence     []InputGTEvidence `json:"evidence,omitempty"`
	Condition    string            `json:"condition,omitempty"`
}

// InputGTProposedMapping is one proposed encoder mapping from discovery findings.
type InputGTProposedMapping struct {
	EncoderType     string `json:"encoderType"`
	Name            string `json:"name"`
	SourceCandidate string `json:"sourceCandidate"`
	Confidence      string `json:"confidence,omitempty"`
	Notes           string `json:"notes,omitempty"`
	Condition       string `json:"condition,omitempty"`
}

// InputGTNormalizedFindings is the normalized candidate payload consumed by inspector/planner/authoring.
type InputGTNormalizedFindings struct {
	SchemaVersion   string                   `json:"schemaVersion,omitempty"`
	MethodVersion   string                   `json:"methodVersion,omitempty"`
	Inputs          []InputGTCandidate       `json:"inputs,omitempty"`
	GroundTruths    []InputGTCandidate       `json:"groundTruths,omitempty"`
	ProposedMapping []InputGTProposedMapping `json:"proposedMapping,omitempty"`
	Unknowns        []string                 `json:"unknowns,omitempty"`
	Comments        string                   `json:"comments,omitempty"`
}

// InputGTFrameworkScore captures framework score pairs used by framework detection.
type InputGTFrameworkScore struct {
	PyTorch    float64 `json:"pytorch,omitempty"`
	TensorFlow float64 `json:"tensorflow,omitempty"`
}

// InputGTFrameworkComponents captures signal-vs-artifact score components.
type InputGTFrameworkComponents struct {
	SignalScores   InputGTFrameworkScore `json:"signalScores,omitempty"`
	ArtifactScores InputGTFrameworkScore `json:"artifactScores,omitempty"`
}

// InputGTFrameworkEvidence captures one evidence item used in framework detection.
type InputGTFrameworkEvidence struct {
	Framework string  `json:"framework,omitempty"`
	Type      string  `json:"type,omitempty"`
	Path      string  `json:"path,omitempty"`
	Detail    string  `json:"detail,omitempty"`
	Weight    float64 `json:"weight,omitempty"`
}

// InputGTFrameworkDetection summarizes framework classification from repository signals.
type InputGTFrameworkDetection struct {
	Candidate  string                     `json:"candidate,omitempty"`
	Confidence string                     `json:"confidence,omitempty"`
	Scores     InputGTFrameworkScore      `json:"scores,omitempty"`
	Components InputGTFrameworkComponents `json:"components,omitempty"`
	Evidence   []InputGTFrameworkEvidence `json:"evidence,omitempty"`
}

// InputGTLeadSignal captures one weighted signal definition used by framework lead extraction.
type InputGTLeadSignal struct {
	ID          string  `json:"id"`
	Framework   string  `json:"framework,omitempty"`
	Description string  `json:"description,omitempty"`
	Weight      float64 `json:"weight,omitempty"`
	Tier        string  `json:"tier,omitempty"`
}

// InputGTLeadSignalOccurrence captures one source-code occurrence for a signal hit.
type InputGTLeadSignalOccurrence struct {
	Line    int    `json:"line"`
	Snippet string `json:"snippet,omitempty"`
}

// InputGTLeadSignalHit captures one scored signal hit for a lead file.
type InputGTLeadSignalHit struct {
	SignalID     string                        `json:"signalId"`
	Framework    string                        `json:"framework,omitempty"`
	Count        int                           `json:"count,omitempty"`
	Contribution float64                       `json:"contribution,omitempty"`
	Occurrences  []InputGTLeadSignalOccurrence `json:"occurrences,omitempty"`
}

// InputGTLeadFile captures scored lead details for one repository file.
type InputGTLeadFile struct {
	Path            string                 `json:"path"`
	Score           float64                `json:"score,omitempty"`
	FrameworkScores InputGTFrameworkScore  `json:"frameworkScores,omitempty"`
	SignalHits      []InputGTLeadSignalHit `json:"signalHits,omitempty"`
}

// InputGTLeadPack is the machine artifact for framework-agnostic lead extraction.
type InputGTLeadPack struct {
	SchemaVersion      string                    `json:"schemaVersion,omitempty"`
	MethodVersion      string                    `json:"methodVersion,omitempty"`
	GeneratedAt        string                    `json:"generatedAt,omitempty"`
	RepoPath           string                    `json:"repoPath,omitempty"`
	PythonFilesScanned int                       `json:"pythonFilesScanned,omitempty"`
	FrameworkDetection InputGTFrameworkDetection `json:"frameworkDetection,omitempty"`
	Signals            []InputGTLeadSignal       `json:"signals,omitempty"`
	Files              []InputGTLeadFile         `json:"files,omitempty"`
	FilesWithHits      int                       `json:"filesWithHits,omitempty"`
	SignalHitCount     int                       `json:"signalHitCount,omitempty"`
}

// InputGTFixtureState captures deterministic discovery stage context for one snapshot.
type InputGTFixtureState struct {
	RepoRoot            string `json:"repoRoot,omitempty"`
	SnapshotID          string `json:"snapshotId,omitempty"`
	WorktreeFingerprint string `json:"worktreeFingerprint,omitempty"`
}

// InputGTAgentPromptBundle captures prompt material used for semantic discovery investigation.
type InputGTAgentPromptBundle struct {
	SystemPrompt              string `json:"systemPrompt,omitempty"`
	UserPrompt                string `json:"userPrompt,omitempty"`
	ReadOnly                  bool   `json:"readOnly,omitempty"`
	LeadPackReadSuccess       bool   `json:"leadPackReadSuccess,omitempty"`
	LeadPackReadInformational bool   `json:"leadPackReadInformational,omitempty"`
}

// InputGTAgentRawOutput captures investigator raw payload + metadata before normalization.
type InputGTAgentRawOutput struct {
	Provider string            `json:"provider,omitempty"`
	Model    string            `json:"model,omitempty"`
	Payload  string            `json:"payload,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// InputGTComparisonReport captures semantic-first comparison details for discovery outputs.
type InputGTComparisonReport struct {
	PrimaryInputSymbols           []string `json:"primaryInputSymbols,omitempty"`
	PrimaryGroundTruthSymbols     []string `json:"primaryGroundTruthSymbols,omitempty"`
	ConditionalInputSymbols       []string `json:"conditionalInputSymbols,omitempty"`
	ConditionalGroundTruthSymbols []string `json:"conditionalGroundTruthSymbols,omitempty"`
	RuntimeInputSymbols           []string `json:"runtimeInputSymbols,omitempty"`
	RuntimeOnlyInputSymbols       []string `json:"runtimeOnlyInputSymbols,omitempty"`
	DiscoveryOnlyInputSymbols     []string `json:"discoveryOnlyInputSymbols,omitempty"`
	Notes                         []string `json:"notes,omitempty"`
}

// InputGTDiscoveryArtifacts captures staged discovery artifacts persisted across the pipeline.
type InputGTDiscoveryArtifacts struct {
	FixtureState       *InputGTFixtureState       `json:"fixtureState,omitempty"`
	LeadPack           *InputGTLeadPack           `json:"leadPack,omitempty"`
	LeadSummary        string                     `json:"leadSummary,omitempty"`
	AgentPromptBundle  *InputGTAgentPromptBundle  `json:"agentPromptBundle,omitempty"`
	AgentRawOutput     *InputGTAgentRawOutput     `json:"agentRawOutput,omitempty"`
	NormalizedFindings *InputGTNormalizedFindings `json:"normalizedFindings,omitempty"`
	ComparisonReport   *InputGTComparisonReport   `json:"comparisonReport,omitempty"`
}

// EncoderMappingContract captures user-confirmed input/GT mapping contract persisted in state.
type EncoderMappingContract struct {
	SourceFingerprint  string    `json:"sourceFingerprint,omitempty"`
	InputSymbols       []string  `json:"inputSymbols,omitempty"`
	GroundTruthSymbols []string  `json:"groundTruthSymbols,omitempty"`
	AcceptedAt         time.Time `json:"acceptedAt,omitempty"`
	Notes              []string  `json:"notes,omitempty"`
}

// IntegrationContracts captures discovered interface symbols from the integration entry file.
type IntegrationContracts struct {
	EntryFile                    string                     `json:"entryFile"`
	LoadModelFunctions           []string                   `json:"loadModelFunctions,omitempty"`
	PreprocessFunctions          []string                   `json:"preprocessFunctions,omitempty"`
	InputEncoders                []string                   `json:"inputEncoders,omitempty"`
	GroundTruthEncoders          []string                   `json:"groundTruthEncoders,omitempty"`
	IntegrationTestFunctions     []string                   `json:"integrationTestFunctions,omitempty"`
	IntegrationTestCalls         []string                   `json:"integrationTestCalls,omitempty"`
	ModelCandidates              []ModelCandidate           `json:"modelCandidates,omitempty"`
	ResolvedModelPath            string                     `json:"resolvedModelPath,omitempty"`
	DiscoveredInputSymbols       []string                   `json:"discoveredInputSymbols,omitempty"`
	DiscoveredGroundTruthSymbols []string                   `json:"discoveredGroundTruthSymbols,omitempty"`
	ConfirmedMapping             *EncoderMappingContract    `json:"confirmedMapping,omitempty"`
	InputGTDiscovery             *InputGTDiscoveryArtifacts `json:"inputGtDiscovery,omitempty"`
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
	LeapYAMLBoundary     string   `json:"leapYamlBoundary,omitempty"`
	SelectedModelPath    string   `json:"selectedModelPath,omitempty"`
	ModelCandidates      []string `json:"modelCandidates,omitempty"`
	DecoratorInventory   []string `json:"decoratorInventory,omitempty"`
	IntegrationTestCalls []string `json:"integrationTestCalls,omitempty"`
	BlockingIssues       []string `json:"blockingIssues,omitempty"`
	ValidationFindings   []string `json:"validationFindings,omitempty"`
}

// AuthoringRecommendation captures deterministic, step-scoped remediation guidance.
type AuthoringRecommendation struct {
	StepID      EnsureStepID `json:"stepId"`
	Target      string       `json:"target,omitempty"`
	Rationale   string       `json:"rationale,omitempty"`
	Candidates  []string     `json:"candidates,omitempty"`
	Constraints []string     `json:"constraints,omitempty"`
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
	Step            EnsureStep                `json:"step"`
	Applied         bool                      `json:"applied"`
	Summary         string                    `json:"summary"`
	Evidence        []EvidenceItem            `json:"evidence,omitempty"`
	Recommendations []AuthoringRecommendation `json:"recommendations,omitempty"`
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
	Passed   bool           `json:"passed"`
	Issues   []Issue        `json:"issues,omitempty"`
	Evidence []EvidenceItem `json:"evidence,omitempty"`
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
	GeneratedAt     time.Time                 `json:"generatedAt"`
	SnapshotID      string                    `json:"snapshotId"`
	Step            EnsureStep                `json:"step"`
	Applied         bool                      `json:"applied"`
	Evidence        []EvidenceItem            `json:"evidence,omitempty"`
	Recommendations []AuthoringRecommendation `json:"recommendations,omitempty"`
	Checks          []VerifiedCheck           `json:"checks,omitempty"`
	Validation      ValidationResult          `json:"validation"`
	Commit          *CommitMetadata           `json:"commit,omitempty"`
	Notes           []string                  `json:"notes,omitempty"`
}
