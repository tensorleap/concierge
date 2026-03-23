package validate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestGuideValidatorSkipsWhenInterpreterIsMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", "print('hello')\n")

	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{},
		astAnalyzer:   fakeIntegrationTestASTAnalyzer{},
	}
	result, err := validator.Run(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath: filepath.Join(repoRoot, ".venv", "bin", "python"),
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Summary.Skipped {
		t.Fatalf("expected skipped summary, got %+v", result.Summary)
	}
	if !strings.Contains(result.Summary.SkipReason, "interpreter") {
		t.Fatalf("expected interpreter skip reason, got %q", result.Summary.SkipReason)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("expected no issues when guide validation is skipped, got %+v", result.Issues)
	}
}

func TestGuideValidatorParsesStatusTableAndTreatsMissingParserAsBestEffort(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout: strings.Join([]string{
						"Warnings (Default use. It is recommended to set values explicitly):",
						" ⚠️ Parameter 'prediction_types' defaults to [] in the following functions: [load_model]. For more information, check docs",
						"",
						"Decorator Name                     | Added to integration",
						"-------------------------------------------------------",
						"tensorleap_preprocess              | ✅",
						"tensorleap_input_encoder           | ✅",
						"tensorleap_load_model              | ✅",
						"tensorleap_integration_test        | ❌",
						"tensorleap_gt_encoder              | ❌",
						"",
						"Some mandatory components have not yet been added to the Integration test. Recommended next interface to add is: tensorleap_integration_test",
					}, "\n"),
				},
				{
					Command: "poetry run python -c ...",
					Stderr:  "Traceback (most recent call last):\nModuleNotFoundError: No module named 'code_loader'\n",
				},
			},
			errs: []error{
				nil,
				errors.New("exit status 1"),
			},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{},
	}

	result, err := validator.Run(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Summary.Skipped {
		t.Fatalf("did not expect guide validation to be skipped: %+v", result.Summary)
	}
	if got := result.Summary.Recommendation.Stage; got != "thin_integration_test" {
		t.Fatalf("expected thin integration test recommendation, got %q", got)
	}
	if len(result.Summary.Local.DefaultWarnings) != 1 {
		t.Fatalf("expected one default warning, got %+v", result.Summary.Local.DefaultWarnings)
	}
	if result.Summary.Parser.Available {
		t.Fatalf("expected parser to be unavailable, got %+v", result.Summary.Parser)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestDecoratorMissing) {
		t.Fatalf("expected integration_test_decorator_missing issue from status row, got %+v", result.Issues)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestMissingRequiredCalls) {
		t.Fatalf("expected integration_test_missing_required_calls issue from gt_encoder status row, got %+v", result.Issues)
	}
	if !hasEvidenceName(result.Evidence, core.GuideEvidenceSummary) {
		t.Fatalf("expected guide summary evidence, got %+v", result.Evidence)
	}
}

func TestGuideValidatorMapsLeapLoaderPayloadFailures(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout: strings.Join([]string{
						"Decorator Name                     | Added to integration",
						"-------------------------------------------------------",
						"tensorleap_preprocess              | ✅",
						"tensorleap_input_encoder           | ✅",
						"tensorleap_load_model              | ✅",
						"tensorleap_integration_test        | ✅",
						"tensorleap_gt_encoder              | ✅",
						"",
						"Successful!",
					}, "\n"),
				},
				{
					Command: "poetry run python -c ...",
					Stdout: strings.Join([]string{
						"{",
						`  "available": true,`,
						`  "isValid": false,`,
						`  "payloads": [`,
						`    {"name":"preprocess","passed":true},`,
						`    {"name":"image","passed":false,"display":{"training":"ValueError: image path is missing"}},`,
						`    {"name":"label","passed":false,"display":{"validation":"ValueError: label tensor has wrong rank"}}`,
						`  ],`,
						`  "setup": {"preprocess":{"trainingLength":4,"validationLength":2},"inputs":[{"name":"image","shape":[224,224,3],"channelDim":-1}]}`,
						"}",
					}, "\n"),
				},
			},
			errs: []error{nil, nil},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{},
	}

	result, err := validator.Run(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Summary.Parser.Available != true {
		t.Fatalf("expected parser to be available, got %+v", result.Summary.Parser)
	}
	if got := result.Summary.Recommendation.Stage; got != "remaining_inputs" {
		t.Fatalf("expected remaining inputs recommendation, got %q", got)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeInputEncoderExecutionFailed) {
		t.Fatalf("expected input-encoder issue, got %+v", result.Issues)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeGTEncoderExecutionFailed) {
		t.Fatalf("expected ground-truth issue, got %+v", result.Issues)
	}
}

func TestGuideValidatorPrefersSpecificPayloadFailureOverGenericParserImportError(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout: strings.Join([]string{
						"Decorator Name                     | Added to integration",
						"-------------------------------------------------------",
						"tensorleap_preprocess              | ❌",
						"tensorleap_input_encoder           | ✅",
						"tensorleap_load_model              | ✅",
						"tensorleap_integration_test        | ✅",
						"tensorleap_gt_encoder              | ✅",
						"",
						"Some mandatory components have not yet been added to the Integration test. Recommended next interface to add is: tensorleap_preprocess",
					}, "\n"),
				},
				{
					Command: "poetry run python -c ...",
					Stdout: strings.Join([]string{
						"{",
						`  "available": true,`,
						`  "isValid": false,`,
						`  "generalError": "Something went wrong. None in file downloads.py, line_number:  14",`,
						`  "payloads": [`,
						`    {"name":"preprocess","passed":false,"display":{"training":"ImportError(\"cannot import name 'LOGGER' from 'ultralytics.utils' (unknown location)\") in file downloads.py, line_number:  14"}}`,
						`  ]`,
						"}",
					}, "\n"),
				},
			},
			errs: []error{nil, nil},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{},
	}

	result, err := validator.Run(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !containsIssueCode(result.Issues, core.IssueCodePreprocessExecutionFailed) {
		t.Fatalf("expected preprocess issue, got %+v", result.Issues)
	}
	if containsIssueCode(result.Issues, core.IssueCodeIntegrationScriptImportFailed) {
		t.Fatalf("did not expect generic integration-script import issue when a preprocess payload failed, got %+v", result.Issues)
	}
	primary, ok := core.SelectPrimaryEnsureStep(result.Issues)
	if !ok {
		t.Fatalf("expected a primary ensure step, got issues %+v", result.Issues)
	}
	if primary.ID != core.EnsureStepPreprocessContract {
		t.Fatalf("expected primary step %q, got %q", core.EnsureStepPreprocessContract, primary.ID)
	}
}

func TestDeriveGuideRecommendationDoesNotMislabelGenericParserFailureAsPreprocess(t *testing.T) {
	summary := core.GuideValidationSummary{
		Local: core.GuideLocalRunSummary{
			StatusRows: []core.GuideStatusRow{
				{Name: "tensorleap_preprocess", Status: "fail"},
				{Name: "tensorleap_input_encoder", Status: "pass"},
				{Name: "tensorleap_load_model", Status: "pass"},
				{Name: "tensorleap_integration_test", Status: "fail"},
			},
		},
		Parser: core.GuideParserRunSummary{
			Available:    true,
			GeneralError: "Something went wrong. PermissionError(13, 'Permission denied') in file pathlib.py, line_number:  1175",
		},
	}

	recommendation := deriveGuideRecommendation(summary)
	if recommendation.Stage != "thin_integration_test" {
		t.Fatalf("expected generic parser failures to fall through to thin integration test, got %+v", recommendation)
	}
}

func TestDeriveGuideRecommendationPrefersGroundTruthStatusOverStaleInputPayloadFailure(t *testing.T) {
	summary := core.GuideValidationSummary{
		LocalStatusTableSupported: true,
		Local: core.GuideLocalRunSummary{
			StatusRows: []core.GuideStatusRow{
				{Name: "tensorleap_preprocess", Status: "pass"},
				{Name: "tensorleap_input_encoder", Status: "pass"},
				{Name: "tensorleap_load_model", Status: "pass"},
				{Name: "tensorleap_integration_test", Status: "pass"},
				{Name: "tensorleap_gt_encoder", Status: "fail"},
			},
		},
		Parser: core.GuideParserRunSummary{
			Available: true,
			Payloads: []core.GuidePayloadSummary{
				{Name: "image", Passed: false, HandlerType: string(guideHandlerInput)},
			},
		},
	}

	recommendation := deriveGuideRecommendation(summary)
	if recommendation.Stage != "ground_truth" {
		t.Fatalf("expected ground-truth recommendation, got %+v", recommendation)
	}
}

func TestGuideValidatorPrefersASTIntegrationTestIssuesOverGenericMappingFailure(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout: strings.Join([]string{
						"Tensorleap_integration_test code flow failed",
						"Decorator Name                     | Added to integration",
						"-------------------------------------------------------",
						"tensorleap_preprocess              | ✅",
						"tensorleap_input_encoder           | ✅",
						"tensorleap_load_model              | ✅",
						"tensorleap_integration_test        | ❌",
					}, "\n"),
				},
				{
					Command: "poetry run python -c ...",
					Stdout:  `{"available":false}`,
				},
			},
			errs: []error{nil, nil},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{
			result: IntegrationTestASTResult{
				Issues: []core.Issue{
					{
						Code:     core.IssueCodeIntegrationTestIllegalBodyLogic,
						Message:  "integration_test should stay declarative",
						Severity: core.SeverityError,
						Scope:    core.IssueScopeIntegrationTest,
					},
				},
			},
		},
	}

	result, err := validator.Run(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeIntegrationTestIllegalBodyLogic) {
		t.Fatalf("expected AST issue, got %+v", result.Issues)
	}
	if containsIssueCode(result.Issues, core.IssueCodeIntegrationTestExecutionFailed) {
		t.Fatalf("did not expect generic mapping-failure issue when AST issue exists, got %+v", result.Issues)
	}
}

func TestGuideValidatorDoesNotAssumePreprocessFailureWhenLegacyCodeLoaderOmitsStatusTable(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout:  "",
				},
				{
					Command: "poetry run python -c ...",
					Stdout: strings.Join([]string{
						"{",
						`  "available": true,`,
						`  "isValid": false,`,
						`  "payloads": [`,
						`    {"name":"preprocess","passed":true},`,
						`    {"name":"image","passed":false,"display":{"training":"ValueError: image path is missing"}}`,
						`  ],`,
						`  "setup": {"preprocess":{"trainingLength":4,"validationLength":2},"inputs":[{"name":"image","shape":[224,224,3],"channelDim":-1}]}`,
						"}",
					}, "\n"),
				},
			},
			errs: []error{nil, nil},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{},
	}

	snapshot := guideValidationSnapshot(t, repoRoot)
	snapshot.RuntimeProfile.CodeLoader = core.CodeLoaderCapabilityState{
		ProbeSucceeded:                true,
		Version:                       "1.0.138",
		SupportsGuideLocalStatusTable: false,
		SupportsCheckDataset:          true,
	}

	result, err := validator.Run(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Summary.Recommendation.Stage; got != "remaining_inputs" {
		t.Fatalf("expected remaining inputs recommendation, got %q", got)
	}
	if result.Summary.LocalStatusTableSupported {
		t.Fatalf("did not expect local status table support, got %+v", result.Summary)
	}
}

func TestGuideValidatorMapsMissingNativeLibraryParserErrorsToRuntimeIssue(t *testing.T) {
	repoRoot := buildGuideValidationRepo(t)
	validator := &GuideValidator{
		runtimeRunner: &fakeGuideRuntimeRunner{
			results: []PythonRuntimeCommandResult{
				{
					Command: "poetry run python leap_integration.py",
					Stdout:  "",
				},
				{
					Command: "poetry run python -c ...",
					Stdout: strings.Join([]string{
						"{",
						`  "available": true,`,
						`  "isValid": false,`,
						`  "generalError": "ImportError('libGL.so.1: cannot open shared object file: No such file or directory')"`,
						"}",
					}, "\n"),
				},
			},
			errs: []error{nil, nil},
		},
		astAnalyzer: fakeIntegrationTestASTAnalyzer{},
	}

	result, err := validator.Run(context.Background(), guideValidationSnapshot(t, repoRoot))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Summary.Recommendation.Stage; got != "runtime_native_dependency" {
		t.Fatalf("expected runtime native dependency recommendation, got %q", got)
	}
	if !containsIssueCode(result.Issues, core.IssueCodeNativeSystemDependencyMissing) {
		t.Fatalf("expected native system dependency issue, got %+v", result.Issues)
	}
}

type fakeGuideRuntimeRunner struct {
	results []PythonRuntimeCommandResult
	errs    []error
	calls   int
}

type fakeIntegrationTestASTAnalyzer struct {
	result IntegrationTestASTResult
	err    error
}

func (f fakeIntegrationTestASTAnalyzer) Analyze(ctx context.Context, snapshot core.WorkspaceSnapshot) (IntegrationTestASTResult, error) {
	_ = ctx
	_ = snapshot
	return f.result, f.err
}

func (f *fakeGuideRuntimeRunner) RunPython(ctx context.Context, snapshot core.WorkspaceSnapshot, args ...string) (PythonRuntimeCommandResult, error) {
	_ = ctx
	_ = snapshot
	_ = args

	index := f.calls
	f.calls++
	if index >= len(f.results) {
		return PythonRuntimeCommandResult{}, nil
	}
	var err error
	if index < len(f.errs) {
		err = f.errs[index]
	}
	return f.results[index], err
}

func buildGuideValidationRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	writeGuideFixtureFile(t, repoRoot, "leap.yaml", "entryFile: leap_integration.py\n")
	writeGuideFixtureFile(t, repoRoot, "leap_integration.py", strings.Join([]string{
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_gt_encoder, tensorleap_input_encoder",
		"",
		"@tensorleap_input_encoder(name=\"image\")",
		"def encode_image(sample_id, preprocess_response):",
		"    return None",
		"",
		"@tensorleap_gt_encoder(name=\"label\")",
		"def encode_label(sample_id, preprocess_response):",
		"    return None",
		"",
	}, "\n"))
	return repoRoot
}

func guideValidationSnapshot(t *testing.T, repoRoot string) core.WorkspaceSnapshot {
	t.Helper()

	interpreterPath := filepath.Join(repoRoot, ".venv", "bin", "python")
	if err := os.MkdirAll(filepath.Dir(interpreterPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(interpreterPath, []byte("#!/usr/bin/env python3\n"), 0o755); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	return core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
		RuntimeProfile: &core.LocalRuntimeProfile{
			Kind:            "poetry",
			InterpreterPath: interpreterPath,
		},
	}
}

func writeGuideFixtureFile(t *testing.T, repoRoot, relativePath, contents string) {
	t.Helper()

	targetPath := filepath.Join(repoRoot, relativePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func hasEvidenceName(evidence []core.EvidenceItem, name string) bool {
	for _, item := range evidence {
		if item.Name == name {
			return true
		}
	}
	return false
}

func TestIssuesFromGuideStatusRowsMandatoryFail(t *testing.T) {
	local := core.GuideLocalRunSummary{
		StatusRows: []core.GuideStatusRow{
			{Name: "tensorleap_preprocess", Status: "pass"},
			{Name: "tensorleap_input_encoder", Status: "fail"},
			{Name: "tensorleap_custom_loss", Status: "fail"},
		},
	}

	issues := issuesFromGuideStatusRows(local)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %+v", len(issues), issues)
	}
	if issues[0].Code != core.IssueCodeIntegrationTestMissingRequiredCalls {
		t.Fatalf("expected integration_test_missing_required_calls issue code for input_encoder, got %q", issues[0].Code)
	}
	if issues[0].Severity != core.SeverityError {
		t.Fatalf("expected error severity, got %q", issues[0].Severity)
	}
	if issues[1].Code != core.IssueCodeIntegrationTestMissingRequiredCalls {
		t.Fatalf("expected integration test missing calls issue code for custom_loss, got %q", issues[1].Code)
	}
}

func TestIssuesFromGuideStatusRowsSkipsOptional(t *testing.T) {
	local := core.GuideLocalRunSummary{
		StatusRows: []core.GuideStatusRow{
			{Name: "tensorleap_custom_metric (optional)", Status: "fail"},
			{Name: "tensorleap_metadata (optional)", Status: "fail"},
			{Name: "tensorleap_custom_visualizer (optional)", Status: "fail"},
		},
	}

	issues := issuesFromGuideStatusRows(local)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for optional decorators, got %d: %+v", len(issues), issues)
	}
}

func TestIssuesFromGuideStatusRowsSkipsWhenMandatoryReady(t *testing.T) {
	local := core.GuideLocalRunSummary{
		MandatoryReady: true,
		StatusRows: []core.GuideStatusRow{
			{Name: "tensorleap_input_encoder", Status: "fail"},
		},
	}

	issues := issuesFromGuideStatusRows(local)
	if len(issues) != 0 {
		t.Fatalf("expected no issues when MandatoryReady, got %d: %+v", len(issues), issues)
	}
}

func TestIssuesFromGuideStatusRowsAllKnownDecorators(t *testing.T) {
	local := core.GuideLocalRunSummary{
		StatusRows: []core.GuideStatusRow{
			{Name: "tensorleap_preprocess", Status: "fail"},
			{Name: "tensorleap_input_encoder", Status: "fail"},
			{Name: "tensorleap_gt_encoder", Status: "fail"},
			{Name: "tensorleap_load_model", Status: "fail"},
			{Name: "tensorleap_integration_test", Status: "fail"},
			{Name: "tensorleap_custom_loss", Status: "fail"},
		},
	}

	issues := issuesFromGuideStatusRows(local)
	if len(issues) != 6 {
		t.Fatalf("expected 6 issues, got %d: %+v", len(issues), issues)
	}

	// input_encoder, gt_encoder, and custom_loss all map to IntegrationTestMissingRequiredCalls,
	// so we check unique codes that must appear at least once.
	expected := map[core.IssueCode]bool{
		core.IssueCodePreprocessFunctionMissing:           false,
		core.IssueCodeIntegrationTestMissingRequiredCalls: false,
		core.IssueCodeLoadModelDecoratorMissing:           false,
		core.IssueCodeIntegrationTestDecoratorMissing:     false,
	}
	for _, issue := range issues {
		expected[issue.Code] = true
	}
	for code, seen := range expected {
		if !seen {
			t.Fatalf("expected issue code %q not found", code)
		}
	}
}
