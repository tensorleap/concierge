package validate

import (
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHarnessIssueMapperMapsHandlerFailures(t *testing.T) {
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "subset_count", Subset: "train", Count: 2},
		{Event: "subset_count", Subset: "validation", Count: 1},
		{Event: "handler_inventory", HandlerKind: "input", Symbol: "image"},
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Subset: "train", SampleID: "0", Status: "failed", Message: "boom"},
	})

	if !hasIssueCode(issues, core.IssueCodeInputEncoderExecutionFailed) {
		t.Fatalf("expected input execution issue, got %+v", issues)
	}
	if !hasIssueCode(issues, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("expected input coverage issue, got %+v", issues)
	}
}

func TestHarnessIssueMapperMapsMandatorySubsetProblems(t *testing.T) {
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "preprocess", Status: "ok"},
		{Event: "subset_count", Subset: "train", Count: 0},
		{Event: "summary", Status: "ok"},
	})

	if !hasIssueCode(issues, core.IssueCodePreprocessSubsetEmpty) {
		t.Fatalf("expected empty subset issue, got %+v", issues)
	}
	if !hasIssueCode(issues, core.IssueCodePreprocessValidationSubsetMissing) {
		t.Fatalf("expected validation missing issue, got %+v", issues)
	}
}

func TestHarnessIssueMapperMapsTypedRuntimeFailures(t *testing.T) {
	finite := false
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "subset_count", Subset: "train", Count: 1},
		{Event: "subset_count", Subset: "validation", Count: 1},
		{Event: "handler_result", HandlerKind: "ground_truth", Symbol: "label", Subset: "validation", SampleID: "1", Status: "dtype_invalid", Message: "unsupported dtype object"},
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Subset: "train", SampleID: "0", Finite: &finite, Status: "non_finite"},
	})

	if !hasIssueCode(issues, core.IssueCodeGTEncoderDTypeInvalid) {
		t.Fatalf("expected gt dtype issue, got %+v", issues)
	}
	if !hasIssueCode(issues, core.IssueCodeInputEncoderNonFiniteValues) {
		t.Fatalf("expected input non-finite issue, got %+v", issues)
	}
}

func TestHarnessIssueMapperDoesNotSynthesizeMissingSubsetsOnRuntimeFailure(t *testing.T) {
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "runtime_failed", Status: "failed", Message: "bootstrap failed"},
	})

	if !hasIssueCode(issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected harness validation issue, got %+v", issues)
	}
	if hasIssueCode(issues, core.IssueCodePreprocessTrainSubsetMissing) {
		t.Fatalf("did not expect train subset issue on bootstrap failure: %+v", issues)
	}
	if hasIssueCode(issues, core.IssueCodePreprocessValidationSubsetMissing) {
		t.Fatalf("did not expect validation subset issue on bootstrap failure: %+v", issues)
	}
}

func TestHarnessIssueMapperDoesNotSynthesizeMissingSubsetsOnPreprocessFailure(t *testing.T) {
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "preprocess", Status: "failed", Message: "boom"},
	})

	if !hasIssueCode(issues, core.IssueCodePreprocessExecutionFailed) {
		t.Fatalf("expected preprocess execution issue, got %+v", issues)
	}
	if hasIssueCode(issues, core.IssueCodePreprocessTrainSubsetMissing) {
		t.Fatalf("did not expect train subset issue on preprocess failure: %+v", issues)
	}
	if hasIssueCode(issues, core.IssueCodePreprocessValidationSubsetMissing) {
		t.Fatalf("did not expect validation subset issue on preprocess failure: %+v", issues)
	}
}

func TestHarnessIssueMapperMapsShapeFailures(t *testing.T) {
	issues := MapHarnessIssues([]HarnessEvent{
		{Event: "subset_count", Subset: "train", Count: 1},
		{Event: "subset_count", Subset: "validation", Count: 1},
		{Event: "handler_result", HandlerKind: "input", Symbol: "image", Subset: "train", SampleID: "0", Status: "shape_invalid", Message: "expected shape [224, 224, 3], got [128, 128, 3]"},
	})

	if !hasIssueCode(issues, core.IssueCodeInputEncoderShapeInvalid) {
		t.Fatalf("expected input shape issue, got %+v", issues)
	}
}
