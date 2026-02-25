package validate

import (
	"context"
	"reflect"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestValidatorFailsOnEmptyStepID(t *testing.T) {
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
