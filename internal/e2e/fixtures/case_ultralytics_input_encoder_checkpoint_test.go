package fixtures

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseUltralyticsInputEncoders_SelectsInputStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "ultralytics_input_encoders")
	modelPath := filepath.Join(repoRoot, ".concierge", "materialized_models", "model.onnx")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", modelPath, err)
	}
	if err := os.WriteFile(modelPath, []byte("fixture placeholder model"), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", modelPath, err)
	}

	_, status, plan := inspectPlanForCase(t, entry, repoRoot)

	assertExpectedIssueCodes(t, status.Issues, entry.ExpectedIssueCodes)
	assertCasePrimaryStep(t, entry, plan)
	if !hasIssueWithSymbol(status.Issues, "image", core.IssueCodeInputEncoderCoverageIncomplete, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("expected input-encoder issue for symbol %q, got %+v", "image", status.Issues)
	}
	if containsAnyIssueCode(status.Issues,
		core.IssueCodeModelAcquisitionRequired,
		core.IssueCodeModelAcquisitionUnresolved,
		core.IssueCodeModelMaterializationOutputMissing,
		core.IssueCodeModelFileMissing,
		core.IssueCodeModelFormatUnsupported,
	) {
		t.Fatalf("expected warmed ultralytics case to avoid model-acquisition blockers, got %+v", status.Issues)
	}
}
