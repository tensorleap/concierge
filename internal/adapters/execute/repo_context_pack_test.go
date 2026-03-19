package execute

import (
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildAgentRepoContextIncludesSelectedModelAndCandidates(t *testing.T) {
	repoRoot := t.TempDir()

	context, err := BuildAgentRepoContext(
		core.EnsureStepPreprocessContract,
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{
				"leap.yaml":           "hash-leap",
				"leap_integration.py": "hash-entry",
				"requirements.txt":    "hash-req",
			},
			SelectedModelPath: "models/selected.onnx",
			RuntimeProfile: &core.LocalRuntimeProfile{
				Kind:              "poetry",
				InterpreterPath:   filepath.ToSlash(filepath.Join(repoRoot, ".venv", "bin", "python")),
				DependenciesReady: true,
				CodeLoaderReady:   true,
				CodeLoader: core.CodeLoaderCapabilityState{
					ProbeSucceeded: true,
					Version:        "1.0.166",
				},
			},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				EntryFile: "leap_integration.py",
				ConfirmedMapping: &core.EncoderMappingContract{
					InputSymbols:       []string{"image"},
					GroundTruthSymbols: []string{"label"},
				},
				ModelCandidates: []core.ModelCandidate{
					{Path: "models/b.onnx", Source: "repo_search"},
					{Path: "models/a.onnx", Source: "repo_search"},
				},
			},
		},
		core.ValidationResult{},
	)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error: %v", err)
	}

	if context.SelectedModelPath != "models/selected.onnx" {
		t.Fatalf("expected selected model path %q, got %q", "models/selected.onnx", context.SelectedModelPath)
	}
	if !reflect.DeepEqual(context.RequiredInputSymbols, []string{"image"}) {
		t.Fatalf("expected required input symbols %+v, got %+v", []string{"image"}, context.RequiredInputSymbols)
	}
	if !reflect.DeepEqual(context.RequiredGroundTruthSymbols, []string{"label"}) {
		t.Fatalf("expected required ground-truth symbols %+v, got %+v", []string{"label"}, context.RequiredGroundTruthSymbols)
	}

	wantCandidates := []string{"models/a.onnx", "models/b.onnx", "models/selected.onnx"}
	if !reflect.DeepEqual(context.ModelCandidates, wantCandidates) {
		t.Fatalf("expected model candidates %+v, got %+v", wantCandidates, context.ModelCandidates)
	}

	if context.EntryFile != "leap_integration.py" {
		t.Fatalf("expected entry file %q, got %q", "leap_integration.py", context.EntryFile)
	}
	if !strings.Contains(context.LeapYAMLBoundary, "leap.yaml present") {
		t.Fatalf("expected leap.yaml boundary summary, got %q", context.LeapYAMLBoundary)
	}
	if context.RuntimeKind != "poetry" {
		t.Fatalf("expected runtime kind %q, got %q", "poetry", context.RuntimeKind)
	}
	if !strings.HasSuffix(context.RuntimeInterpreter, "/.venv/bin/python") {
		t.Fatalf("expected runtime interpreter path in context, got %q", context.RuntimeInterpreter)
	}
	if context.RuntimeStatus != "dependencies ready; code_loader import succeeded (1.0.166)" {
		t.Fatalf("unexpected runtime status %q", context.RuntimeStatus)
	}
}

func TestBuildAgentRepoContextDeterministicOrderingAndTruncation(t *testing.T) {
	repoRoot := t.TempDir()

	modelCandidates := []core.ModelCandidate{
		{Path: "models/j.onnx"},
		{Path: "models/i.onnx"},
		{Path: "models/h.onnx"},
		{Path: "models/g.onnx"},
		{Path: "models/f.onnx"},
		{Path: "models/e.onnx"},
		{Path: "models/d.onnx"},
		{Path: "models/c.onnx"},
		{Path: "models/b.onnx"},
		{Path: "models/a.onnx"},
	}

	statusA := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			EntryFile:       "leap_integration.py",
			ModelCandidates: modelCandidates,
		},
		Issues: modelIssuesDescending(15),
	}
	statusB := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			EntryFile:       "leap_integration.py",
			ModelCandidates: reverseModelCandidates(modelCandidates),
		},
		Issues: reverseIssues(modelIssuesDescending(15)),
	}

	validationA := core.ValidationResult{
		Passed: false,
		Issues: modelValidationIssuesDescending(15),
	}
	validationB := core.ValidationResult{
		Passed: false,
		Issues: reverseIssues(modelValidationIssuesDescending(15)),
	}

	snapshot := core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: repoRoot},
		FileHashes: map[string]string{
			"leap.yaml":           "hash-leap",
			"leap_integration.py": "hash-entry",
		},
	}

	contextA, err := BuildAgentRepoContext(core.EnsureStepModelContract, snapshot, statusA, validationA)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error for contextA: %v", err)
	}
	contextB, err := BuildAgentRepoContext(core.EnsureStepModelContract, snapshot, statusB, validationB)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error for contextB: %v", err)
	}

	if !reflect.DeepEqual(contextA, contextB) {
		t.Fatalf("expected deterministic context output,\nA=%+v\nB=%+v", contextA, contextB)
	}

	if len(contextA.ModelCandidates) != maxRepoContextModelCandidates {
		t.Fatalf("expected model candidates to be truncated to %d, got %d (%+v)", maxRepoContextModelCandidates, len(contextA.ModelCandidates), contextA.ModelCandidates)
	}
	wantCandidates := []string{
		"models/a.onnx",
		"models/b.onnx",
		"models/c.onnx",
		"models/d.onnx",
		"models/e.onnx",
		"models/f.onnx",
		"models/g.onnx",
		"models/h.onnx",
	}
	if !reflect.DeepEqual(contextA.ModelCandidates, wantCandidates) {
		t.Fatalf("expected truncated sorted candidates %+v, got %+v", wantCandidates, contextA.ModelCandidates)
	}

	if len(contextA.BlockingIssues) != maxRepoContextBlockingIssues {
		t.Fatalf("expected blocking issues to be truncated to %d, got %d", maxRepoContextBlockingIssues, len(contextA.BlockingIssues))
	}
	if len(contextA.ValidationFindings) != maxRepoContextValidationFindings {
		t.Fatalf("expected validation findings to be truncated to %d, got %d", maxRepoContextValidationFindings, len(contextA.ValidationFindings))
	}
}

func TestBuildAgentRepoContextAllowsModelStepWithoutResolvedCandidates(t *testing.T) {
	repoRoot := t.TempDir()

	context, err := BuildAgentRepoContext(
		core.EnsureStepModelContract,
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{"leap.yaml": "hash-leap"},
		},
		core.IntegrationStatus{},
		core.ValidationResult{},
	)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error: %v", err)
	}
	if context.SelectedModelPath != "" {
		t.Fatalf("expected empty selected model path, got %q", context.SelectedModelPath)
	}
	if len(context.ModelCandidates) != 0 {
		t.Fatalf("expected no model candidates, got %+v", context.ModelCandidates)
	}
}

func TestBuildAgentRepoContextIncludesAcquisitionLeads(t *testing.T) {
	repoRoot := t.TempDir()

	context, err := BuildAgentRepoContext(
		core.EnsureStepModelAcquisition,
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{"leap.yaml": "hash-leap"},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				ModelAcquisition: &core.ModelAcquisitionArtifacts{
					PassiveLeads: []core.ModelCandidate{{Path: "weights/model.pt"}},
					AcquisitionLeads: []string{
						"project_config.yaml",
						"docker/Dockerfile-cpu -> https://example.com/releases/model.onnx",
					},
				},
			},
		},
		core.ValidationResult{},
	)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error: %v", err)
	}

	want := []string{
		"docker/Dockerfile-cpu -> https://example.com/releases/model.onnx",
		"project_config.yaml",
		"weights/model.pt",
	}
	if !reflect.DeepEqual(context.ModelAcquisitionLeads, want) {
		t.Fatalf("expected acquisition leads %+v, got %+v", want, context.ModelAcquisitionLeads)
	}
}

func TestBuildAgentRepoContextIncludesSelectedModelAcquisitionPlan(t *testing.T) {
	repoRoot := t.TempDir()

	context, err := BuildAgentRepoContext(
		core.EnsureStepModelAcquisition,
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			ModelAcquisitionPlan: &core.ModelAcquisitionPlan{
				Strategy:           "user_clarified_strategy",
				DefaultChoice:      "export from weights/best.pt",
				ExpectedOutputPath: ".concierge/materialized_models/model.onnx",
				RuntimeInvocation:  []string{"poetry", "run", "python", "tools/export_model.py"},
			},
		},
		core.IntegrationStatus{},
		core.ValidationResult{},
	)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error: %v", err)
	}
	if context.ModelAcquisitionPlan == nil {
		t.Fatal("expected repo context to include model acquisition plan")
	}
	if context.ModelAcquisitionPlan.Strategy != "user_clarified_strategy" {
		t.Fatalf("expected strategy %q, got %+v", "user_clarified_strategy", context.ModelAcquisitionPlan)
	}
	if context.ModelAcquisitionPlan.ExpectedOutputPath != ".concierge/materialized_models/model.onnx" {
		t.Fatalf("expected expected output path %q, got %+v", ".concierge/materialized_models/model.onnx", context.ModelAcquisitionPlan)
	}
}

func TestBuildAgentRepoContextPrefersPrimaryDiscoverySymbolsWhenMappingMissing(t *testing.T) {
	repoRoot := t.TempDir()

	context, err := BuildAgentRepoContext(
		core.EnsureStepInputEncoders,
		core.WorkspaceSnapshot{
			Repository: core.RepositoryState{Root: repoRoot},
			FileHashes: map[string]string{"leap.yaml": "hash-leap"},
		},
		core.IntegrationStatus{
			Contracts: &core.IntegrationContracts{
				InputGTDiscovery: &core.InputGTDiscoveryArtifacts{
					ComparisonReport: &core.InputGTComparisonReport{
						PrimaryInputSymbols:       []string{"image"},
						PrimaryGroundTruthSymbols: []string{"bbox"},
					},
				},
				DiscoveredInputSymbols:       []string{"image", "images"},
				DiscoveredGroundTruthSymbols: []string{"bbox", "labels"},
			},
		},
		core.ValidationResult{},
	)
	if err != nil {
		t.Fatalf("BuildAgentRepoContext returned error: %v", err)
	}
	if !reflect.DeepEqual(context.RequiredInputSymbols, []string{"image"}) {
		t.Fatalf("expected primary input symbols to win, got %+v", context.RequiredInputSymbols)
	}
	if len(context.RequiredGroundTruthSymbols) != 0 {
		t.Fatalf("expected out-of-scope ground-truth symbols to be hidden for input step, got %+v", context.RequiredGroundTruthSymbols)
	}
}

func modelIssuesDescending(count int) []core.Issue {
	issues := make([]core.Issue, 0, count)
	for i := count; i >= 1; i-- {
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeModelFileMissing,
			Message:  "model blocker " + strconv.Itoa(i),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
	}
	return issues
}

func modelValidationIssuesDescending(count int) []core.Issue {
	issues := make([]core.Issue, 0, count)
	for i := count; i >= 1; i-- {
		issues = append(issues, core.Issue{
			Code:     core.IssueCodeModelFormatUnsupported,
			Message:  "model validation finding " + strconv.Itoa(i),
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeModel,
		})
	}
	return issues
}

func reverseModelCandidates(values []core.ModelCandidate) []core.ModelCandidate {
	reversed := append([]core.ModelCandidate(nil), values...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}

func reverseIssues(values []core.Issue) []core.Issue {
	reversed := append([]core.Issue(nil), values...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}
