package execute

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestScopePolicyForPreprocessUsesOnlyPreprocessSection(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepPreprocessContract, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "preprocess_contract")
	assertNotContains(t, policy.DomainSections, "load_model_contract")
	assertContainsSubstring(t, policy.ForbiddenAreas, "@tensorleap_input_encoder")
	assertContainsSubstring(t, policy.ForbiddenAreas, "@tensorleap_integration_test")
	assertContainsSubstring(t, policy.ForbiddenAreas, ".concierge")
	assertContainsSubstring(t, policy.ForbiddenAreas, "install packages")
	assertContainsSubstring(t, policy.ForbiddenAreas, "global site-packages")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "placeholder sample IDs")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "hard-code installed package defaults")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "/datasets")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "pip install")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "repo-supported dataset resolver")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "generic repo assets")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "system directories")
	assertContainsSubstring(t, policy.StopAndAskTriggers, "vendored dataset/cache artifacts")
}

func TestPolicyForPreprocessUsesOnlyPreprocessSection(t *testing.T) {
	TestScopePolicyForPreprocessUsesOnlyPreprocessSection(t)
}

func TestScopePolicyForInputEncodersExcludesGTAndIntegrationTestSections(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepInputEncoders, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "input_encoder_contract")
	assertNotContains(t, policy.DomainSections, "ground_truth_encoder_contract")
	assertNotContains(t, policy.DomainSections, "integration_test_wiring_contract")
	assertContainsSubstring(t, policy.RequiredOutcomes, "exact Tensorleap symbol names")
	assertContainsSubstring(t, policy.RequiredOutcomes, "sample_id")
}

func TestPolicyForInputEncodersExcludesGTAndIntegrationTestSections(t *testing.T) {
	TestScopePolicyForInputEncodersExcludesGTAndIntegrationTestSections(t)
}

func TestScopePolicyForModelContractUsesLoadModelSection(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepModelContract, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "load_model_contract")
	assertNotContains(t, policy.DomainSections, "preprocess_contract")
	assertContainsSubstring(t, policy.ForbiddenAreas, "training/business logic")
}

func TestResolveAgentAllowedFilesTruncatesModelCandidates(t *testing.T) {
	snapshot := core.WorkspaceSnapshot{
		SelectedModelPath: "models/selected.onnx",
	}
	status := core.IntegrationStatus{
		Contracts: &core.IntegrationContracts{
			ResolvedModelPath: "models/resolved.onnx",
			ModelCandidates: []core.ModelCandidate{
				{Path: "models/a.onnx"},
				{Path: "models/b.onnx"},
				{Path: "models/c.onnx"},
				{Path: "models/d.onnx"},
				{Path: "models/e.onnx"},
				{Path: "models/f.onnx"},
				{Path: "models/g.onnx"},
				{Path: "models/h.onnx"},
				{Path: "models/i.onnx"},
				{Path: "models/j.onnx"},
			},
		},
	}

	allowed := resolveAgentAllowedFiles(snapshot, status)

	assertContains(t, allowed, "leap.yaml")
	assertContains(t, allowed, "leap_integration.py")
	assertContains(t, allowed, "models/selected.onnx")
	assertContains(t, allowed, "models/resolved.onnx")
	assertContains(t, allowed, "models/a.onnx")
	assertContains(t, allowed, "models/h.onnx")
	assertNotContains(t, allowed, "models/i.onnx")
	assertNotContains(t, allowed, "models/j.onnx")
}

func TestScopePolicyForIntegrationTestUsesNarrowWiringSection(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepIntegrationTestWiring, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "integration_test_wiring_contract")
	assertContainsSubstring(t, policy.ForbiddenAreas, "@tensorleap_input_encoder")
	assertContainsSubstring(t, policy.RequiredOutcomes, "Repair @tensorleap_integration_test")
}

func TestScopePolicyForStepReturnsErrorWhenScopeCannotBeResolved(t *testing.T) {
	_, err := PolicyForStep(core.EnsureStepUploadPush, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err == nil {
		t.Fatal("expected unresolved scope policy error")
	}
	if got := core.KindOf(err); got != core.KindStepNotApplicable {
		t.Fatalf("expected error kind %q, got %q (err=%v)", core.KindStepNotApplicable, got, err)
	}
}

func TestPolicyForStepReturnsErrorWhenScopeCannotBeResolved(t *testing.T) {
	TestScopePolicyForStepReturnsErrorWhenScopeCannotBeResolved(t)
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected %q in %+v", want, values)
}

func assertNotContains(t *testing.T, values []string, disallowed string) {
	t.Helper()
	for _, value := range values {
		if value == disallowed {
			t.Fatalf("did not expect %q in %+v", disallowed, values)
		}
	}
}

func assertContainsSubstring(t *testing.T, values []string, wantSubstring string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(value, wantSubstring) {
			return
		}
	}
	t.Fatalf("expected substring %q in %+v", wantSubstring, values)
}
