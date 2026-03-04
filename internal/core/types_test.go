package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultStagesAreDeterministicAndImmutable(t *testing.T) {
	stages := DefaultStages()
	expected := []Stage{
		StageSnapshot,
		StageInspect,
		StagePlan,
		StageExecute,
		StageValidate,
		StageReport,
	}

	if len(stages) != len(expected) {
		t.Fatalf("expected %d stages, got %d", len(expected), len(stages))
	}

	for i := range expected {
		if stages[i] != expected[i] {
			t.Fatalf("expected stage[%d]=%q, got %q", i, expected[i], stages[i])
		}
	}

	stages[0] = "mutated"
	fresh := DefaultStages()
	if fresh[0] != StageSnapshot {
		t.Fatalf("expected default stages to remain immutable, got %q", fresh[0])
	}
}

func TestIntegrationStatusReady(t *testing.T) {
	if !(IntegrationStatus{}).Ready() {
		t.Fatal("expected empty status to be ready")
	}

	withMissing := IntegrationStatus{Missing: []string{"leap.yaml"}}
	if withMissing.Ready() {
		t.Fatal("expected status with missing artifacts to be not ready")
	}

	withIssue := IntegrationStatus{Issues: []Issue{{Code: IssueCodeUnknown, Message: "boom", Severity: SeverityError}}}
	if withIssue.Ready() {
		t.Fatal("expected status with issues to be not ready")
	}
}

func TestIssueLocationIsOptional(t *testing.T) {
	issue := Issue{
		Code:     IssueCodePreprocessFunctionMissing,
		Message:  "preprocess function is required but missing",
		Severity: SeverityError,
		Scope:    IssueScopePreprocess,
	}

	raw, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(raw), "\"location\"") {
		t.Fatalf("did not expect location when omitted, got: %s", string(raw))
	}

	issue.Location = &IssueLocation{
		Path:   "leap_binder.py",
		Line:   42,
		Column: 3,
		Symbol: "preprocess",
	}

	raw, err = json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal with location failed: %v", err)
	}
	if !strings.Contains(string(raw), "\"location\"") {
		t.Fatalf("expected location when provided, got: %s", string(raw))
	}
}

func TestExecutionResultRecommendationsOptionalJSON(t *testing.T) {
	result := ExecutionResult{
		Step:    EnsureStep{ID: EnsureStepModelContract},
		Applied: false,
		Summary: "model recommendation ready",
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal without recommendations failed: %v", err)
	}
	if strings.Contains(string(raw), "\"recommendations\"") {
		t.Fatalf("did not expect recommendations when omitted, got: %s", string(raw))
	}

	result.Recommendations = []AuthoringRecommendation{
		{
			StepID:     EnsureStepModelContract,
			Target:     "model/demo.h5",
			Rationale:  "single_supported_candidate",
			Candidates: []string{"model/demo.h5"},
		},
	}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal with recommendations failed: %v", err)
	}
	if !strings.Contains(string(raw), "\"recommendations\"") {
		t.Fatalf("expected recommendations when provided, got: %s", string(raw))
	}
}
