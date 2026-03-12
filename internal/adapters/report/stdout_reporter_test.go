package report

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestReporterWritesSingleLineSummary(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapYAML},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepRepositoryContext,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepRepositoryContext),
				Status: core.CheckStatusPass,
			},
			{
				StepID:   core.EnsureStepLeapYAML,
				Label:    core.HumanEnsureStepLabel(core.EnsureStepLeapYAML),
				Status:   core.CheckStatusFail,
				Blocking: true,
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeLeapYAMLMissing,
						Message:  "leap.yaml is required at repository root",
						Severity: core.SeverityError,
						Scope:    core.IssueScopeLeapYAML,
					},
				},
			},
		},
		Validation: core.ValidationResult{
			Passed: false,
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeLeapYAMLMissing,
					Message:  "leap.yaml is required at repository root",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeLeapYAML,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Integration Checklist",
		"leap.yaml should be present and valid (missing step)",
		"Missing integration step: leap.yaml should be present and valid",
		"leap.yaml is required at repository root",
		"I can help with this step interactively and will ask before making any changes.",
		"Changes: No changes were applied.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(snippet)) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
	if strings.Contains(output, "Upload prerequisites are satisfied") {
		t.Fatalf("expected output to omit future unchecked checks, got %q", output)
	}
}

func TestReporterShowsNextStepsWhenChecksPass(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepComplete},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepRepositoryContext,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepRepositoryContext),
				Status: core.CheckStatusPass,
			},
			{
				StepID: core.EnsureStepLeapYAML,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepLeapYAML),
				Status: core.CheckStatusPass,
			},
		},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Verified checks passed.",
		"Next steps:",
		"run `leap push` from the repository root.",
		tensorleapUploadGuideURL,
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
}

func TestReporterShowsWarningStateWhenChecksHaveWarnings(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapCLIAuth},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepLeapCLIAuth,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepLeapCLIAuth),
				Status: core.CheckStatusWarning,
			},
		},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"⚠ Leap CLI should be installed and authenticated",
		"Warning: Leap CLI should be installed and authenticated",
		"I can help with this step interactively and will ask before making any changes.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
	if strings.Contains(output, "Missing integration step:") {
		t.Fatalf("did not expect missing-step heading for warning-only checks, got %q", output)
	}
}

func TestReporterStopsAtFirstWarningAndPrintsWarningDetails(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepComplete},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepRepositoryContext,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepRepositoryContext),
				Status: core.CheckStatusPass,
			},
			{
				StepID: core.EnsureStepLeapCLIAuth,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepLeapCLIAuth),
				Status: core.CheckStatusWarning,
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeLeapCLINotAuthenticated,
						Message:  "leap CLI is not authenticated",
						Severity: core.SeverityWarning,
						Scope:    core.IssueScopeCLI,
					},
				},
			},
			{
				StepID: core.EnsureStepLeapYAML,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepLeapYAML),
				Status: core.CheckStatusPass,
			},
		},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"☑ Repository context is ready",
		"⚠ Leap CLI should be installed and authenticated",
		"Warning: Leap CLI should be installed and authenticated",
		"Details:",
		"leap CLI is not authenticated",
		"This warning is advisory in the current run, so I did not apply a fix automatically.",
		"Next step: run `leap auth login`, then rerun `concierge run`.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
	if strings.Contains(output, "leap.yaml is present and valid") {
		t.Fatalf("expected output to stop at first warning, got %q", output)
	}
	if strings.Contains(output, "I can help with this step interactively and will ask before making any changes.") {
		t.Fatalf("did not expect interactive-help claim for warning-only completed run, got %q", output)
	}
}

func TestReporterShowsManualGuidanceWhenExecutorCannotApplyFix(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepLeapCLIAuth},
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "stub"},
		},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepLeapCLIAuth,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepLeapCLIAuth),
				Status: core.CheckStatusWarning,
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeLeapCLINotFound,
						Message:  "leap CLI was not found in PATH",
						Severity: core.SeverityWarning,
						Scope:    core.IssueScopeCLI,
					},
				},
			},
		},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Warning: Leap CLI should be installed and authenticated",
		"leap CLI was not found in PATH",
		"I cannot apply an automated fix for this check in the current run.",
		"Next step: install the Leap CLI, then run `leap --version` and `leap auth login`, and rerun `concierge run`.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
	if strings.Contains(output, "I can help with this step interactively and will ask before making any changes.") {
		t.Fatalf("did not expect interactive-help claim when executor mode is stub, got %q", output)
	}
}

func TestReporterShowsGuideValidationMilestoneAndRecommendation(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	summary := core.GuideValidationSummary{
		Local: core.GuideLocalRunSummary{
			Successful: true,
			DefaultWarnings: []string{
				"Parameter 'prediction_types' defaults to [] in the following functions: [load_model]. For more information, check docs",
			},
		},
		Parser: core.GuideParserRunSummary{
			Attempted: true,
			Available: true,
			IsValid:   true,
		},
		Recommendation: core.GuideRecommendation{
			Stage:   "wider_sample_coverage",
			Message: "Next recommended milestone: expand from first-sample success to a few training and validation samples.",
		},
	}
	rawSummary, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	err = reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepComplete},
		Checks: []core.VerifiedCheck{
			{
				StepID: core.EnsureStepRepositoryContext,
				Label:  core.HumanEnsureStepLabel(core.EnsureStepRepositoryContext),
				Status: core.CheckStatusPass,
			},
		},
		Evidence: []core.EvidenceItem{
			{Name: core.GuideEvidenceSummary, Value: string(rawSummary)},
		},
		Validation: core.ValidationResult{Passed: true},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Guide validation:",
		"First-sample milestone: `Successful!` was reached.",
		"Default warnings remain: Parameter 'prediction_types' defaults to [] in the following functions: [load_model].",
		"Next recommended milestone: expand from first-sample success to a few training and validation samples.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
}

func TestReporterShowsIntegrationTestRepairGuidance(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-ast",
		Step:       core.EnsureStep{ID: core.EnsureStepComplete},
		Checks: []core.VerifiedCheck{
			{
				StepID:   core.EnsureStepIntegrationTestContract,
				Label:    core.HumanEnsureStepRequirementLabel(core.EnsureStepIntegrationTestContract),
				Status:   core.CheckStatusFail,
				Blocking: true,
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
						Message:  "integration_test should stay declarative",
						Severity: core.SeverityError,
						Scope:    core.IssueScopeIntegrationTest,
					},
				},
			},
		},
		Validation: core.ValidationResult{
			Passed: false,
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
					Message:  "integration_test should stay declarative",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeIntegrationTest,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Missing integration step: Integration test wiring should be complete",
		"integration_test should stay declarative",
		"keep `@tensorleap_integration_test` thin and declarative",
		"Do not read sample/preprocess data directly or add batch dimensions manually inside the integration test body.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
}

func TestReporterShowsPoetryInstallGuidanceWhenRuntimeNeedsManualSetup(t *testing.T) {
	var sink strings.Builder
	reporter := NewStdoutReporter(&sink)

	err := reporter.Report(context.Background(), core.IterationReport{
		SnapshotID: "snapshot-123",
		Step:       core.EnsureStep{ID: core.EnsureStepPythonRuntime},
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "self_service"},
		},
		Checks: []core.VerifiedCheck{
			{
				StepID:   core.EnsureStepPythonRuntime,
				Label:    core.HumanEnsureStepRequirementLabel(core.EnsureStepPythonRuntime),
				Status:   core.CheckStatusFail,
				Blocking: true,
				Issues: []core.Issue{
					{
						Code:     core.IssueCodePoetryEnvironmentUnresolved,
						Message:  "Concierge could not find a working Poetry environment for this project. Run `poetry install` in this repo first.",
						Severity: core.SeverityError,
						Scope:    core.IssueScopeEnvironment,
					},
				},
			},
		},
		Validation: core.ValidationResult{Passed: false},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}

	output := sink.String()
	expectedSnippets := []string{
		"Missing integration step: Poetry environment should be available and have the required packages",
		"Concierge could not find a working Poetry environment for this project.",
		"I cannot apply an automated fix for this check in the current run.",
		"Next step: run `poetry install` in this project.",
		"If `poetry env info --executable` still does not print a Python path, run `poetry env use <python>`, then rerun `concierge run`.",
		"You do not need to start Concierge with `poetry run`; Concierge will use the Poetry environment automatically.",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected output to contain %q, got %q", snippet, output)
		}
	}
	if strings.Contains(output, "I can help with this step interactively and will ask before making any changes.") {
		t.Fatalf("did not expect interactive-help claim for self-service runtime setup, got %q", output)
	}
}

func TestReporterReturnsWriteError(t *testing.T) {
	reporter := NewStdoutReporter(failingWriter{})

	err := reporter.Report(context.Background(), core.IterationReport{SnapshotID: "snapshot-1"})
	if err == nil {
		t.Fatal("expected write error")
	}
	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("expected wrapped write error, got %v", err)
	}
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (f failingWriter) Write(p []byte) (int, error) {
	_ = p
	return 0, errWriteFailed
}
