package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	bundledscripts "github.com/tensorleap/concierge/scripts"
)

const (
	// HarnessEnableEnvVar controls whether runtime harness execution is enabled.
	HarnessEnableEnvVar = "CONCIERGE_ENABLE_HARNESS"
)

const defaultHarnessTimeout = 120 * time.Second
const defaultHarnessScriptPath = "scripts/harness_runtime.py"

// HarnessRunResult captures parsed output from a harness invocation.
type HarnessRunResult struct {
	Enabled  bool
	Events   []HarnessEvent
	Issues   []core.Issue
	Evidence []core.EvidenceItem
}

// HarnessRunner invokes an optional Python harness and parses NDJSON output.
type HarnessRunner struct {
	timeout       time.Duration
	scriptPath    string
	getEnv        func(string) string
	runtimeRunner *PythonRuntimeRunner
}

// NewHarnessRunner creates a harness runner with default command wiring.
func NewHarnessRunner() *HarnessRunner {
	return &HarnessRunner{
		timeout:       defaultHarnessTimeout,
		scriptPath:    defaultHarnessScriptPath,
		getEnv:        os.Getenv,
		runtimeRunner: NewPythonRuntimeRunner(),
	}
}

// Run executes the harness when enabled and parses its NDJSON output.
func (r *HarnessRunner) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (HarnessRunResult, error) {
	r.ensureDefaults()

	if !harnessEnabled(r.getEnv(HarnessEnableEnvVar)) {
		return HarnessRunResult{Enabled: false}, nil
	}

	scriptPath, err := r.resolveScriptPath()
	if err != nil {
		return HarnessRunResult{}, err
	}
	runDir := strings.TrimSpace(snapshot.Repository.Root)
	if runDir == "" {
		runDir = "."
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	commandResult, err := r.runtimeRunner.RunPython(
		runCtx,
		snapshot,
		scriptPath,
		"--repo-root",
		runDir,
		"--entry-file",
		"leap_integration.py",
		"--sample-budget",
		"3",
	)
	runtimeEvidence := []core.EvidenceItem{
		{Name: "runtime.command", Value: commandResult.Command},
		{Name: "runtime.stdout", Value: commandResult.Stdout},
		{Name: "runtime.stderr", Value: commandResult.Stderr},
	}
	if err != nil {
		return HarnessRunResult{
			Enabled: true,
			Issues: []core.Issue{{
				Code:     core.IssueCodeHarnessValidationFailed,
				Message:  fmt.Sprintf("harness execution failed: %s", err.Error()),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeValidation,
			}},
			Evidence: append(runtimeEvidence, harnessRuntimeProvenanceEvidence(snapshot)...),
		}, nil
	}

	parseResult, err := ParseHarnessEvents([]byte(commandResult.Stdout))
	if err != nil {
		evidence := append(runtimeEvidence, harnessRuntimeProvenanceEvidence(snapshot)...)
		if len(parseResult.Noise) > 0 {
			evidence = append(evidence, core.EvidenceItem{
				Name:  "harness.stdout_noise",
				Value: strings.Join(parseResult.Noise, "\n"),
			})
		}
		return HarnessRunResult{
			Enabled: true,
			Issues: []core.Issue{{
				Code:     core.IssueCodeHarnessValidationFailed,
				Message:  fmt.Sprintf("harness output parse failed: %s", err.Error()),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeValidation,
			}},
			Evidence: evidence,
		}, nil
	}

	events := parseResult.Events
	issues := MapHarnessIssues(events)

	evidence := append(runtimeEvidence, harnessRuntimeProvenanceEvidence(snapshot)...)
	if len(parseResult.Noise) > 0 {
		evidence = append(evidence, core.EvidenceItem{
			Name:  "harness.stdout_noise",
			Value: strings.Join(parseResult.Noise, "\n"),
		})
	}

	summaryJSON, err := json.Marshal(harnessEventSummary(events))
	if err != nil {
		return HarnessRunResult{}, core.WrapError(core.KindUnknown, "validate.harness.summary", err)
	}
	evidence = append(evidence, core.EvidenceItem{Name: "harness.summary.json", Value: string(summaryJSON)})

	return HarnessRunResult{
		Enabled:  true,
		Events:   events,
		Issues:   issues,
		Evidence: evidence,
	}, nil
}

func (r *HarnessRunner) ensureDefaults() {
	if r.timeout <= 0 {
		r.timeout = defaultHarnessTimeout
	}
	if strings.TrimSpace(r.scriptPath) == "" {
		r.scriptPath = defaultHarnessScriptPath
	}
	if r.getEnv == nil {
		r.getEnv = os.Getenv
	}
	if r.runtimeRunner == nil {
		r.runtimeRunner = NewPythonRuntimeRunner()
	}
}

func (r *HarnessRunner) resolveScriptPath() (string, error) {
	configuredPath := strings.TrimSpace(r.scriptPath)
	if configuredPath == "" {
		configuredPath = defaultHarnessScriptPath
	}

	if filepath.Clean(configuredPath) == filepath.Clean(defaultHarnessScriptPath) {
		scriptPath, err := bundledscripts.HarnessRuntimePath()
		if err != nil {
			return "", core.WrapError(core.KindUnknown, "validate.harness.script_materialize", err)
		}
		return scriptPath, nil
	}

	scriptPath, err := filepath.Abs(configuredPath)
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "validate.harness.script_abs", err)
	}

	if _, err := os.Stat(scriptPath); err != nil {
		return "", core.WrapError(core.KindUnknown, "validate.harness.script_stat", err)
	}

	return scriptPath, nil
}

func harnessEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off", "no", "disabled":
		return false
	default:
		return true
	}
}

type harnessSummary struct {
	SubsetCounts map[string]int      `json:"subsetCounts,omitempty"`
	SampledIDs   map[string][]string `json:"sampledIds,omitempty"`
}

func harnessEventSummary(events []HarnessEvent) harnessSummary {
	summary := harnessSummary{
		SubsetCounts: map[string]int{},
		SampledIDs:   map[string][]string{},
	}
	for _, event := range events {
		switch normalizeHarnessEvent(event.Event) {
		case "subset_count":
			summary.SubsetCounts[normalizeHarnessSubset(event.Subset)] = event.Count
		case "sample_selected":
			subset := normalizeHarnessSubset(event.Subset)
			summary.SampledIDs[subset] = append(summary.SampledIDs[subset], strings.TrimSpace(event.SampleID))
		}
	}
	return summary
}

func harnessRuntimeProvenanceEvidence(snapshot core.WorkspaceSnapshot) []core.EvidenceItem {
	profile := snapshot.RuntimeProfile
	if profile == nil {
		return []core.EvidenceItem{
			{Name: "runtime.poetry_version", Value: strings.TrimSpace(snapshot.Runtime.PoetryVersion)},
			{Name: "runtime.interpreter_path", Value: strings.TrimSpace(snapshot.Runtime.ResolvedInterpreter)},
			{Name: "runtime.python_version", Value: strings.TrimSpace(snapshot.Runtime.ResolvedPythonVersion)},
		}
	}

	return []core.EvidenceItem{
		{Name: "runtime.poetry_version", Value: firstNonEmpty(strings.TrimSpace(profile.PoetryVersion), strings.TrimSpace(snapshot.Runtime.PoetryVersion))},
		{Name: "runtime.interpreter_path", Value: firstNonEmpty(strings.TrimSpace(profile.InterpreterPath), strings.TrimSpace(snapshot.Runtime.ResolvedInterpreter))},
		{Name: "runtime.python_version", Value: firstNonEmpty(strings.TrimSpace(profile.PythonVersion), strings.TrimSpace(snapshot.Runtime.ResolvedPythonVersion))},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
