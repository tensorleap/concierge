package fixtures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseUltralyticsGTEncoders_SelectsGTStep(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	entry, repoRoot := cloneCaseRepoForTest(t, "ultralytics_gt_encoders")
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
	if !hasIssueWithSymbol(status.Issues, "classes", core.IssueCodeGTEncoderCoverageIncomplete, core.IssueCodeGTEncoderMissing) {
		t.Fatalf("expected GT-encoder issue for symbol %q, got %+v", "classes", status.Issues)
	}
	if containsAnyIssueCode(status.Issues,
		core.IssueCodeInputEncoderCoverageIncomplete,
		core.IssueCodeInputEncoderMissing,
	) {
		t.Fatalf("expected warmed ultralytics GT case to preserve input-encoder coverage, got %+v", status.Issues)
	}
}

func TestFixtureCaseUltralyticsGTEncoders_UsesInputEncoderWarmBase(t *testing.T) {
	patchPath := filepath.Join(repoRootFromRuntime(t), "fixtures", "cases", "patches", "ultralytics_gt_encoders.patch")
	raw, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	rawContent := string(raw)
	addedContent := patchAddedContent(rawContent)

	if !strings.Contains(rawContent, "@tensorleap_integration_test()") {
		t.Fatalf("expected ultralytics GT-encoders patch to keep @tensorleap_integration_test, got:\n%s", rawContent)
	}
	if !strings.Contains(rawContent, "return None") {
		t.Fatalf("expected ultralytics GT-encoders patch to use a thin integration_test scaffold, got:\n%s", rawContent)
	}
	if !strings.Contains(rawContent, "subset.sample_ids[:5]") {
		t.Fatalf("expected ultralytics GT-encoders patch to use the deterministic __main__ scaffold loop, got:\n%s", rawContent)
	}
	if strings.Contains(rawContent, "-@tensorleap_input_encoder('image', channel_dim=1)") {
		t.Fatalf("expected ultralytics GT-encoders patch to preserve the warmed input encoder, got:\n%s", rawContent)
	}
	if !strings.Contains(rawContent, "-@tensorleap_gt_encoder('classes')") {
		t.Fatalf("expected ultralytics GT-encoders patch to remove the GT encoder decorator from the warm base, got:\n%s", rawContent)
	}
	if strings.Contains(addedContent, "@tensorleap_gt_encoder") {
		t.Fatalf("expected ultralytics GT-encoders patch to avoid re-adding GT encoder registrations, got:\n%s", addedContent)
	}
	if strings.Contains(addedContent, "binder_gt_encoder") {
		t.Fatalf("expected ultralytics GT-encoders patch to avoid GT encoder wiring in the warm base, got:\n%s", addedContent)
	}
}
