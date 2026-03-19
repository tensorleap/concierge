package inspect

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPreprocessDetectorEmitsMissingFunctionIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    return None",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodePreprocessFunctionMissing)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
	if issue.Scope != core.IssueScopePreprocess {
		t.Fatalf("expected preprocess scope, got %q", issue.Scope)
	}

	step, ok := core.PreferredEnsureStepForIssueCode(issue.Code)
	if !ok {
		t.Fatalf("expected preferred step for issue %q", issue.Code)
	}
	if step.ID != core.EnsureStepPreprocessContract {
		t.Fatalf("expected preferred step %q, got %q", core.EnsureStepPreprocessContract, step.ID)
	}
}

func TestPreprocessDetectorEmitsInvalidSignatureIssue(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data(dataset):",
		"    return []",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	issue, ok := firstIssueByCode(status.Issues, core.IssueCodePreprocessResponseInvalid)
	if !ok {
		t.Fatalf("expected %q issue, got %+v", core.IssueCodePreprocessResponseInvalid, status.Issues)
	}
	if !strings.Contains(issue.Message, "must not accept parameters") {
		t.Fatalf("expected invalid-signature message, got %q", issue.Message)
	}
	if issue.Location == nil || issue.Location.Symbol != "preprocess_data" {
		t.Fatalf("expected preprocess symbol location, got %+v", issue.Location)
	}
}

func TestPreprocessDetectorDoesNotFlagValidDefinitions(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_integration.py", "print('binder')\n")
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess, tensorleap_load_model",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_data():",
		"    responses = []",
		"    return responses",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return 'model/demo.h5'",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/demo.h5", "binary")

	status := inspectStatus(t, root)

	if hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodePreprocessResponseInvalid) {
		t.Fatalf("did not expect %q issue, got %+v", core.IssueCodePreprocessResponseInvalid, status.Issues)
	}
}

func TestPreprocessDetectorAcceptsBinderHostedPreprocessDefinition(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_binder.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func_leap():",
		"    responses = []",
		"    return responses",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "leap_integration.py", strings.Join([]string{
		"from leap_binder import preprocess_func_leap",
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_integration_test",
		"",
		"@tensorleap_integration_test()",
		"def run_flow():",
		"    return preprocess_func_leap()",
		"",
	}, "\n"))

	status := inspectStatus(t, root)

	if hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("did not expect %q issue for binder-hosted preprocess, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
	if hasIssueCode(status.Issues, core.IssueCodePreprocessResponseInvalid) {
		t.Fatalf("did not expect %q issue for binder-hosted preprocess, got %+v", core.IssueCodePreprocessResponseInvalid, status.Issues)
	}
}

func TestPreprocessDetectorAcceptsUltralyticsStyleBinderImport(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, "leap.yaml", "entryFile: leap_integration.py\n")
	writeFixtureFile(t, root, "leap_binder.py", strings.Join([]string{
		"from typing import List, Dict, Union",
		"from code_loader.contract.datasetclasses import PreprocessResponse, DataStateType",
		"from code_loader.inner_leap_binder.leapbinder_decorators import (tensorleap_preprocess, tensorleap_gt_encoder,",
		"                                                                 tensorleap_input_encoder)",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func_leap() -> List[PreprocessResponse]:",
		"    responses = []",
		"    return responses",
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
		"    return preprocess_func_leap()",
		"",
	}, "\n"))
	writeFixtureFile(t, root, "model/demo.h5", "binary")

	status := inspectStatus(t, root)

	if hasIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf("did not expect %q issue for ultralytics-style binder import, got %+v", core.IssueCodePreprocessFunctionMissing, status.Issues)
	}
}
