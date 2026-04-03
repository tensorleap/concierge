package validate

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestIntegrationTestASTAnalyzerReportsMissingRequiredCalls(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestMissingRequiredCalls) {
		t.Fatalf("expected missing required call issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "required input")
	assertContainsIssueMessage(t, result.Issues, "load_model")
}

func TestIntegrationTestASTAnalyzerFlagsLoadModelWithoutInference(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    _ = image",
		"    model = load_model()",
		"    _ = model",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestMissingRequiredCalls) {
		t.Fatalf("expected missing-required-calls issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "never executes model inference")
}

func TestIntegrationTestASTAnalyzerFlagsUnusedModelPredictions(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    model = load_model()",
		"    predictions = model(image)",
		"    _ = model",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestMissingRequiredCalls) {
		t.Fatalf("expected missing-required-calls issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "never consumes model outputs")
}

func TestIntegrationTestASTAnalyzerFlagsHelperCallsDatasetAccessAndManualBatching(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"import numpy as np",
		"",
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    model = load_model()",
		"    encoded = image_input(sample_id, preprocess_response)",
		"    batch = np.expand_dims(encoded, 0)",
		"    helper(batch)",
		"    return preprocess_response.data['row']",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestManualBatchManipulation) {
		t.Fatalf("expected manual-batch issue, got %+v", result.Issues)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestCallsUnknownInterfaces) {
		t.Fatalf("expected unknown-call issue, got %+v", result.Issues)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestDirectDatasetAccess) {
		t.Fatalf("expected direct-dataset-access issue, got %+v", result.Issues)
	}
}

func TestIntegrationTestASTAnalyzerFlagsTempMappingTransposePattern(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"import numpy as np",
		"",
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    model = load_model()",
		"    input_name = model.get_inputs()[0].name",
		"    image_input = np.expand_dims(image.transpose(2, 0, 1), axis=0)",
		"    return model.run(None, {input_name: image_input})",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestManualBatchManipulation) {
		t.Fatalf("expected manual-batch issue, got %+v", result.Issues)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("expected illegal-body-logic issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "batch dimension automatically")
	assertContainsIssueMessage(t, result.Issues, "external-library transforms directly")
}

func TestIntegrationTestASTAnalyzerAllowsPredictionIndexing(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(idx, subset):",
		"    model = load_model()",
		"    encoded = image_input(idx, subset)",
		"    preds = model(encoded)",
		"    _ = preds[0]",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("did not expect illegal-body-logic issue for prediction indexing, got %+v", result.Issues)
	}
	if containsIssueCode(result.Issues, core.IssueCodeIntegrationTestCallsUnknownInterfaces) {
		t.Fatalf("did not expect unknown-call issue for model inference path, got %+v", result.Issues)
	}
}

func TestIntegrationTestASTAnalyzerAllowsPredictionsThreadedIntoDecoratedSurfaces(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_gt_encoder(name='labels')",
		"def label_targets(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_custom_loss(name='detections')",
		"def detection_loss(predictions, labels):",
		"    return predictions",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    model = load_model()",
		"    predictions = model(image)",
		"    labels = label_targets(sample_id, preprocess_response)",
		"    detection_loss(predictions, labels)",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	assertNotContainsIssueMessage(t, result.Issues, "never executes model inference")
	assertNotContainsIssueMessage(t, result.Issues, "never consumes model outputs")
}

func TestIntegrationTestASTAnalyzerRejectsCallingRawOnnxSessionAsCallable(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"import onnxruntime as ort",
		"",
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model() -> ort.InferenceSession:",
		"    return ort.InferenceSession('model.onnx')",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    model = load_model()",
		"    predictions = model(image)",
		"    _ = predictions[0]",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("expected illegal-body-logic issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "raw ONNX Runtime session as a callable")
}

func TestIntegrationTestASTAnalyzerAllowsOnnxSessionRunPattern(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"import onnxruntime as ort",
		"",
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model() -> ort.InferenceSession:",
		"    return ort.InferenceSession('model.onnx')",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess_response):",
		"    image = image_input(sample_id, preprocess_response)",
		"    model = load_model()",
		"    input_name = model.get_inputs()[0].name",
		"    predictions = model.run(None, {input_name: image})",
		"    _ = predictions[0]",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("did not expect illegal-body-logic issue for ONNX model.run pattern, got %+v", result.Issues)
	}
	if containsIssueCode(result.Issues, core.IssueCodeIntegrationTestCallsUnknownInterfaces) {
		t.Fatalf("did not expect unknown-call issue for ONNX model.run pattern, got %+v", result.Issues)
	}
	assertNotContainsIssueMessage(t, result.Issues, "never executes model inference")
	assertNotContainsIssueMessage(t, result.Issues, "never consumes model outputs")
}

func TestIntegrationTestASTAnalyzerRejectsIndexingNonPredictions(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for AST analyzer tests")
	}

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"@tensorleap_input_encoder(name='image')",
		"def image_input(sample_id, preprocess_response):",
		"    return sample_id",
		"",
		"@tensorleap_load_model()",
		"def load_model():",
		"    return None",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(idx, subset):",
		"    encoded = image_input(idx, subset)",
		"    _ = encoded[0]",
		"    return None",
	}, "\n"))

	analyzer := &IntegrationTestASTAnalyzer{runtimeRunner: scriptRuntimeRunner(t)}
	result, err := analyzer.Analyze(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("expected illegal-body-logic issue, got %+v", result.Issues)
	}
	assertContainsIssueMessage(t, result.Issues, "indexing is only allowed on model predictions")
}

type livePythonRuntimeRunner struct {
	t *testing.T
}

func scriptRuntimeRunner(t *testing.T) *livePythonRuntimeRunner {
	t.Helper()
	return &livePythonRuntimeRunner{t: t}
}

func (r *livePythonRuntimeRunner) RunPython(ctx context.Context, snapshot core.WorkspaceSnapshot, args ...string) (PythonRuntimeCommandResult, error) {
	r.t.Helper()
	command := exec.CommandContext(ctx, "python3", args...)
	command.Dir = snapshot.Repository.Root
	output, err := command.CombinedOutput()
	result := PythonRuntimeCommandResult{
		Command: "python3 " + strings.Join(args, " "),
		Stdout:  strings.TrimSpace(string(output)),
		Stderr:  "",
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func assertContainsIssueMessage(t *testing.T, issues []core.Issue, want string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue.Message, want) {
			return
		}
	}
	t.Fatalf("expected one issue to contain %q, got %+v", want, issues)
}

func assertNotContainsIssueMessage(t *testing.T, issues []core.Issue, unwanted string) {
	t.Helper()
	for _, issue := range issues {
		if strings.Contains(issue.Message, unwanted) {
			t.Fatalf("did not expect one issue to contain %q, got %+v", unwanted, issues)
		}
	}
}
