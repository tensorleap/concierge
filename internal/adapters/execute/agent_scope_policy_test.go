package execute

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestScopePolicyForPreprocessIncludesModelLoadAndPreprocessSections(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepPreprocessContract, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "preprocess_contract")
	assertContains(t, policy.DomainSections, "load_model_contract")
	assertContainsSubstring(t, policy.ForbiddenAreas, "@tensorleap_input_encoder")
	assertContainsSubstring(t, policy.ForbiddenAreas, "@tensorleap_integration_test")
}

func TestPolicyForPreprocessIncludesModelLoadAndPreprocessSections(t *testing.T) {
	TestScopePolicyForPreprocessIncludesModelLoadAndPreprocessSections(t)
}

func TestScopePolicyForInputEncodersExcludesGTAndIntegrationTestSections(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepInputEncoders, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
	if err != nil {
		t.Fatalf("PolicyForStep returned error: %v", err)
	}

	assertContains(t, policy.DomainSections, "input_encoder_contract")
	assertNotContains(t, policy.DomainSections, "ground_truth_encoder_contract")
	assertNotContains(t, policy.DomainSections, "integration_test_wiring_contract")
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

func TestScopePolicyForIntegrationTestUsesNarrowWiringSection(t *testing.T) {
	policy, err := PolicyForStep(core.EnsureStepIntegrationTestContract, core.WorkspaceSnapshot{}, core.IntegrationStatus{})
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
