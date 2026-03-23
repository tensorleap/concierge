package inspect

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestInputEncoderDetectorEmitsMissingEncoderIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    load_model()",
		"    preprocess_data()",
		"    encode_image()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image"}, nil)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeInputEncoderMissing)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if issue.Scope != core.IssueScopeInputEncoder {
		t.Fatalf("expected scope %q, got %q", core.IssueScopeInputEncoder, issue.Scope)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "image") {
		t.Fatalf("expected symbol detail in issue message, got %q", issue.Message)
	}
	if strings.Contains(strings.ToLower(issue.Message), "symbol") {
		t.Fatalf("expected user-facing name wording, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "image" {
		t.Fatalf("expected symbol location %q, got %+v", "image", issue.Location)
	}

	step, ok := core.PreferredEnsureStepForIssueCode(issue.Code)
	if !ok {
		t.Fatalf("expected preferred step for issue %q", issue.Code)
	}
	if step.ID != core.EnsureStepInputEncoders {
		t.Fatalf("expected preferred step %q, got %q", core.EnsureStepInputEncoders, step.ID)
	}
}

func TestInputEncoderDetectorEmitsCoverageIncompleteIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_input_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_image()",
		"    encode_meta()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image", "meta"}, nil)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
	if !strings.Contains(strings.ToLower(issue.Message), "meta") {
		t.Fatalf("expected missing symbol detail in message, got %q", issue.Message)
	}
	if strings.Contains(strings.ToLower(issue.Message), "symbol") {
		t.Fatalf("expected user-facing name wording, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "meta" {
		t.Fatalf("expected location symbol %q, got %+v", "meta", issue.Location)
	}
}

func TestInputEncoderDetectorNoFalsePositiveWhenCoverageComplete(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_preprocess, tensorleap_input_encoder, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def encode_image():",
		"    return 1",
		"",
		"@tensorleap_input_encoder('meta')",
		"def encode_meta():",
		"    return 1",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    encode_image()",
		"    encode_meta()",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image", "meta"}, nil)

	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
}

func TestInputEncoderDetectorAcceptsBinderHostedEncoderDefinition(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_binder.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_input_encoder, tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func_leap():",
		"    return []",
		"",
		"@tensorleap_input_encoder('image')",
		"def input_encoder(idx, preprocess):",
		"    return 1",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from leap_binder import input_encoder, preprocess_func_leap",
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    load_model()",
		"    preprocess_func_leap()",
		"    input_encoder(0, None)",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image"}, nil)

	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("did not expect %q issue for binder-hosted encoder, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("did not expect %q issue for binder-hosted encoder, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
}

func TestInputEncoderDetectorAcceptsUltralyticsStyleBinderImport(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "model/demo.h5", "binary\n")
	writeFixtureFile(t, root, "leap_binder.py", strings.Join([]string{
		"from typing import List, Dict, Union",
		"import numpy as np",
		"from code_loader.contract.datasetclasses import PreprocessResponse, DataStateType",
		"from code_loader.inner_leap_binder.leapbinder_decorators import (tensorleap_preprocess, tensorleap_gt_encoder,",
		"                                                                 tensorleap_input_encoder)",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func_leap() -> List[PreprocessResponse]:",
		"    return []",
		"",
		"@tensorleap_input_encoder('image', channel_dim=1)",
		"def input_encoder(idx: int, preprocess: PreprocessResponse) -> np.ndarray:",
		"    return np.zeros((1, 3, 640, 640), dtype=np.float32)",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.contract.datasetclasses import SamplePreprocessResponse",
		"from leap_binder import (input_encoder, preprocess_func_leap, gt_encoder,",
		"                         loss, gt_bb_decoder, image_visualizer, bb_decoder,",
		"                         cost, metadata_per_img, ious, confusion_matrix_metric)",
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_load_model, tensorleap_integration_test",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
		"@tensorleap_integration_test()",
		"def check_custom_test_mapping(idx, subset):",
		"    image = input_encoder(idx, subset)",
		"    return image",
		"",
	}, "\n"))

	status := inspectStatusWithConfirmedMapping(t, root, []string{"image"}, nil)

	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderMissing) {
		t.Fatalf("did not expect %q issue for ultralytics-style binder import, got %+v", core.IssueCodeInputEncoderMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("did not expect %q issue for ultralytics-style binder import, got %+v", core.IssueCodeInputEncoderCoverageIncomplete, status.Issues)
	}
}

func inspectStatusWithConfirmedMapping(
	t *testing.T,
	root string,
	inputSymbols []string,
	groundTruthSymbols []string,
) core.IntegrationStatus {
	t.Helper()
	snapshot := snapshotForRoot(root)
	snapshot.ConfirmedEncoderMapping = &core.EncoderMappingContract{
		InputSymbols:       append([]string(nil), inputSymbols...),
		GroundTruthSymbols: append([]string(nil), groundTruthSymbols...),
	}

	inspector := NewBaselineInspector()
	status, err := inspector.Inspect(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	return status
}
