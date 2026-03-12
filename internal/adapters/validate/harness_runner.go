package validate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	// HarnessEnableEnvVar controls whether runtime harness execution is enabled.
	HarnessEnableEnvVar = "CONCIERGE_ENABLE_HARNESS"
)

const defaultHarnessTimeout = 120 * time.Second

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
		scriptPath:    filepath.Join("scripts", "harness_runtime.py"),
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
	if err != nil {
		return HarnessRunResult{}, core.WrapError(core.KindUnknown, "validate.harness.run", err)
	}

	events, err := ParseHarnessEvents([]byte(commandResult.Stdout))
	if err != nil {
		return HarnessRunResult{}, err
	}
	issues := MapHarnessIssues(events)

	summaryJSON, err := json.Marshal(harnessEventSummary(events))
	if err != nil {
		return HarnessRunResult{}, core.WrapError(core.KindUnknown, "validate.harness.summary", err)
	}

	return HarnessRunResult{
		Enabled: true,
		Events:  events,
		Issues:  issues,
		Evidence: append(
			[]core.EvidenceItem{
				{Name: "runtime.command", Value: commandResult.Command},
				{Name: "runtime.stdout", Value: commandResult.Stdout},
				{Name: "runtime.stderr", Value: commandResult.Stderr},
			},
			append(harnessRuntimeProvenanceEvidence(snapshot), core.EvidenceItem{Name: "harness.summary.json", Value: string(summaryJSON)})...,
		),
	}, nil
}

func (r *HarnessRunner) ensureDefaults() {
	if r.timeout <= 0 {
		r.timeout = defaultHarnessTimeout
	}
	if strings.TrimSpace(r.scriptPath) == "" {
		r.scriptPath = filepath.Join("scripts", "harness_runtime.py")
	}
	if r.getEnv == nil {
		r.getEnv = os.Getenv
	}
	if r.runtimeRunner == nil {
		r.runtimeRunner = NewPythonRuntimeRunner()
	}
}

func (r *HarnessRunner) resolveScriptPath() (string, error) {
	scriptPath, err := filepath.Abs(strings.TrimSpace(r.scriptPath))
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
