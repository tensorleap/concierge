package validate

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestValidatorFailsOnEmptyStepID(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "0")
	validator := NewBaselineValidator()

	result, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected validation to fail for empty step ID")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected one issue, got %+v", result.Issues)
	}
	if result.Issues[0].Code != core.IssueCodeHarnessValidationFailed {
		t.Fatalf("expected issue code %q, got %q", core.IssueCodeHarnessValidationFailed, result.Issues[0].Code)
	}
}

func TestValidatorPassesForStubExecution(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "0")
	validator := NewBaselineValidator()

	validation, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{
		Step:    core.EnsureStep{ID: core.EnsureStepLeapYAML},
		Applied: false,
		Summary: "not implemented",
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !validation.Passed {
		t.Fatal("expected validation to pass for stub execution")
	}
	if len(validation.Issues) != 1 {
		t.Fatalf("expected one note issue, got %+v", validation.Issues)
	}
	if validation.Issues[0].Code != core.IssueCodeUnknown {
		t.Fatalf("expected issue code %q, got %q", core.IssueCodeUnknown, validation.Issues[0].Code)
	}
	if validation.Issues[0].Severity != core.SeverityInfo {
		t.Fatalf("expected severity %q, got %q", core.SeverityInfo, validation.Issues[0].Severity)
	}
}

func TestValidatorDeterministicOutput(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "0")
	validator := NewBaselineValidator()
	execution := core.ExecutionResult{
		Step:    core.EnsureStep{ID: core.EnsureStepIntegrationScript},
		Applied: false,
		Summary: "not implemented",
	}

	first, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, execution)
	if err != nil {
		t.Fatalf("first Validate returned error: %v", err)
	}
	second, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, execution)
	if err != nil {
		t.Fatalf("second Validate returned error: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic output, got first=%+v second=%+v", first, second)
	}
}

func TestValidatorIngestsHarnessIssuesAndHeuristics(t *testing.T) {
	validator := newBaselineValidatorWithHarness(fakeHarnessRunner{
		result: HarnessRunResult{
			Enabled: true,
			Events: []HarnessEvent{
				{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "same"},
				{Event: "handler_result", HandlerKind: "input", Symbol: "image", Fingerprint: "same"},
			},
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeHarnessValidationFailed,
					Message:  "harness failed",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeValidation,
				},
			},
		},
	})

	validation, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{
		Step: core.EnsureStep{ID: core.EnsureStepHarnessValidation},
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if validation.Passed {
		t.Fatalf("expected validation to fail when harness returns error issues, got %+v", validation)
	}
	if !containsIssueCode(validation.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected harness issue in %+v", validation.Issues)
	}
	if !containsIssueCode(validation.Issues, core.IssueCodeSuspiciousConstantInputs) {
		t.Fatalf("expected heuristic issue in %+v", validation.Issues)
	}
}

func TestValidatorSkipsHarnessUntilGuideFirstSamplePasses(t *testing.T) {
	validator := newBaselineValidatorWithGuideHarness(fakeGuideRunner{
		result: GuideValidationResult{
			Summary: core.GuideValidationSummary{},
		},
	}, fakeHarnessRunner{
		result: HarnessRunResult{
			Enabled: true,
			Issues: []core.Issue{{
				Code:     core.IssueCodeHarnessValidationFailed,
				Message:  "should not run",
				Severity: core.SeverityError,
				Scope:    core.IssueScopeValidation,
			}},
		},
	})

	validation, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{
		Step: core.EnsureStep{ID: core.EnsureStepHarnessValidation},
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if containsIssueCode(validation.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("did not expect harness issue when guide gate is not ready: %+v", validation.Issues)
	}
	if !hasEvidenceName(validation.Evidence, "harness.skip_reason") {
		t.Fatalf("expected harness skip evidence in %+v", validation.Evidence)
	}
}

func TestValidatorReturnsErrorWhenHarnessFails(t *testing.T) {
	validator := newBaselineValidatorWithHarness(fakeHarnessRunner{
		err: errors.New("harness crashed"),
	})

	_, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{
		Step: core.EnsureStep{ID: core.EnsureStepHarnessValidation},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidatorIngestsGuideIssuesAndEvidence(t *testing.T) {
	validator := newBaselineValidatorWithGuideHarness(fakeGuideRunner{
		result: GuideValidationResult{
			Issues: []core.Issue{
				{
					Code:     core.IssueCodeIntegrationTestExecutionFailed,
					Message:  "mapping mode failed",
					Severity: core.SeverityError,
					Scope:    core.IssueScopeIntegrationTest,
				},
			},
			Evidence: []core.EvidenceItem{
				{Name: core.GuideEvidenceSummary, Value: `{"recommendation":{"stage":"thin_integration_test"}}`},
			},
		},
	}, nil)

	validation, err := validator.Validate(context.Background(), core.WorkspaceSnapshot{}, core.ExecutionResult{
		Step: core.EnsureStep{ID: core.EnsureStepIntegrationTestContract},
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if validation.Passed {
		t.Fatalf("expected guide error to fail validation, got %+v", validation)
	}
	if !containsIssueCode(validation.Issues, core.IssueCodeIntegrationTestExecutionFailed) {
		t.Fatalf("expected guide issue in %+v", validation.Issues)
	}
	if !hasEvidenceName(validation.Evidence, core.GuideEvidenceSummary) {
		t.Fatalf("expected guide evidence in %+v", validation.Evidence)
	}
}

type fakeHarnessRunner struct {
	result HarnessRunResult
	err    error
}

func (f fakeHarnessRunner) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (HarnessRunResult, error) {
	_ = ctx
	_ = snapshot
	return f.result, f.err
}

type fakeGuideRunner struct {
	result GuideValidationResult
	err    error
}

func (f fakeGuideRunner) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (GuideValidationResult, error) {
	_ = ctx
	_ = snapshot
	return f.result, f.err
}

func containsIssueCode(issues []core.Issue, code core.IssueCode) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
