package fixtures

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFixtureCaseUltralyticsInputEncoders_UsesThinIntegrationTestScaffold(t *testing.T) {
	patchPath := filepath.Join(repoRootFromRuntime(t), "fixtures", "cases", "patches", "ultralytics_input_encoders.patch")
	raw, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	rawContent := string(raw)
	addedContent := patchAddedContent(rawContent)

	if !strings.Contains(rawContent, "@tensorleap_integration_test()") {
		t.Fatalf("expected ultralytics input-encoders patch to keep @tensorleap_integration_test, got:\n%s", rawContent)
	}
	if !strings.Contains(rawContent, "return None") {
		t.Fatalf("expected ultralytics input-encoders patch to use a thin integration_test scaffold, got:\n%s", rawContent)
	}
	if !strings.Contains(rawContent, "subset.sample_ids[:5]") {
		t.Fatalf("expected ultralytics input-encoders patch to use the deterministic __main__ scaffold loop, got:\n%s", rawContent)
	}
	if strings.Contains(addedContent, "model.run(") {
		t.Fatalf("expected ultralytics input-encoders patch to avoid legacy model.run wiring before encoder repair, got:\n%s", addedContent)
	}
	if strings.Contains(addedContent, "binder_input_encoder(") {
		t.Fatalf("expected ultralytics input-encoders patch to avoid legacy binder_input_encoder wiring before encoder repair, got:\n%s", addedContent)
	}
	if strings.Contains(addedContent, "@tensorleap_gt_encoder") {
		t.Fatalf("expected ultralytics input-encoders patch to avoid GT encoder registrations before the GT step, got:\n%s", addedContent)
	}
	if strings.Contains(addedContent, "binder_gt_encoder") {
		t.Fatalf("expected ultralytics input-encoders patch to avoid GT encoder wiring before the GT step, got:\n%s", addedContent)
	}
}

func patchAddedContent(raw string) string {
	var builder strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "+++") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			builder.WriteString(strings.TrimPrefix(line, "+"))
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
