package core

import "testing"

func TestBuildVerifiedChecksUsesExplicitVerificationSignalsOnly(t *testing.T) {
	snapshot := WorkspaceSnapshot{}
	inspectIssues := []Issue{
		{
			Code:     IssueCodeLeapYAMLMissing,
			Message:  "leap.yaml is required at repository root",
			Severity: SeverityError,
			Scope:    IssueScopeLeapYAML,
		},
	}

	checks := BuildVerifiedChecks(snapshot, inspectIssues, nil, EnsureStepLeapYAML)
	if len(checks) == 0 {
		t.Fatal("expected non-empty checks")
	}

	leapCheck, ok := findCheckByStep(checks, EnsureStepLeapYAML)
	if !ok {
		t.Fatalf("expected %q check row", EnsureStepLeapYAML)
	}
	if leapCheck.Status != CheckStatusFail {
		t.Fatalf("expected leap.yaml check to fail, got %q", leapCheck.Status)
	}
	if !leapCheck.Blocking {
		t.Fatal("expected leap.yaml check to be marked blocking")
	}

	if _, exists := findCheckByStep(checks, EnsureStepUploadPush); exists {
		t.Fatalf("did not expect %q to be rendered", EnsureStepUploadPush)
	}
	if _, exists := findCheckByStep(checks, EnsureStepUploadReadiness); exists {
		t.Fatalf("did not expect %q to be rendered", EnsureStepUploadReadiness)
	}
}

func TestBuildVerifiedChecksIncludesProbeBasedWarnings(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Runtime: RuntimeState{ProbeRan: true},
	}
	inspectIssues := []Issue{
		{
			Code:     IssueCodePythonVersionUnsupported,
			Message:  "python 3.7 is unsupported; python 3.8+ is recommended",
			Severity: SeverityWarning,
			Scope:    IssueScopeEnvironment,
		},
	}

	checks := BuildVerifiedChecks(snapshot, inspectIssues, nil, "")
	pythonCheck, ok := findCheckByStep(checks, EnsureStepPythonRuntime)
	if !ok {
		t.Fatalf("expected %q check row", EnsureStepPythonRuntime)
	}
	if pythonCheck.Status != CheckStatusWarning {
		t.Fatalf("expected python check warning, got %q", pythonCheck.Status)
	}
}

func TestBuildVerifiedChecksIncludesIssueDrivenSteps(t *testing.T) {
	inspectIssues := []Issue{
		{
			Code:     IssueCodeLeapSecretMissing,
			Message:  "required leap secret is missing",
			Severity: SeverityWarning,
			Scope:    IssueScopeSecrets,
		},
	}

	checks := BuildVerifiedChecks(WorkspaceSnapshot{}, inspectIssues, nil, "")
	secretsCheck, ok := findCheckByStep(checks, EnsureStepSecretsContext)
	if !ok {
		t.Fatalf("expected %q check row from issue mapping", EnsureStepSecretsContext)
	}
	if secretsCheck.Status != CheckStatusWarning {
		t.Fatalf("expected secrets check warning, got %q", secretsCheck.Status)
	}
}

func TestVerifiedCheckStatusHelpers(t *testing.T) {
	checks := []VerifiedCheck{
		{StepID: EnsureStepRepositoryContext, Status: CheckStatusPass},
		{StepID: EnsureStepPythonRuntime, Status: CheckStatusWarning},
		{StepID: EnsureStepLeapYAML, Status: CheckStatusFail},
	}

	if !HasFailingVerifiedChecks(checks) {
		t.Fatal("expected failing check detection")
	}
	if !HasWarningVerifiedChecks(checks) {
		t.Fatal("expected warning check detection")
	}
}

func TestVisibleChecksForFlowStopsAtFirstFail(t *testing.T) {
	checks := []VerifiedCheck{
		{StepID: EnsureStepRepositoryContext, Status: CheckStatusPass},
		{StepID: EnsureStepLeapCLIAuth, Status: CheckStatusWarning},
		{StepID: EnsureStepLeapYAML, Status: CheckStatusFail},
		{StepID: EnsureStepModelContract, Status: CheckStatusPass},
	}

	visible := VisibleChecksForFlow(checks)
	if len(visible) != 3 {
		t.Fatalf("expected 3 visible checks through first fail, got %d", len(visible))
	}
	if visible[2].StepID != EnsureStepLeapYAML {
		t.Fatalf("expected first fail step to be last visible item, got %q", visible[2].StepID)
	}

	attention, ok := FirstAttentionCheck(visible)
	if !ok {
		t.Fatal("expected attention check")
	}
	if attention.StepID != EnsureStepLeapYAML {
		t.Fatalf("expected fail to be attention check, got %q", attention.StepID)
	}
}

func TestVisibleChecksForFlowStopsAtFirstWarningWhenNoFail(t *testing.T) {
	checks := []VerifiedCheck{
		{StepID: EnsureStepRepositoryContext, Status: CheckStatusPass},
		{StepID: EnsureStepLeapCLIAuth, Status: CheckStatusWarning},
		{StepID: EnsureStepLeapYAML, Status: CheckStatusPass},
	}

	visible := VisibleChecksForFlow(checks)
	if len(visible) != 2 {
		t.Fatalf("expected 2 visible checks through first warning, got %d", len(visible))
	}
	if visible[1].StepID != EnsureStepLeapCLIAuth {
		t.Fatalf("expected warning step to be last visible item, got %q", visible[1].StepID)
	}

	attention, ok := FirstAttentionCheck(visible)
	if !ok {
		t.Fatal("expected attention check")
	}
	if attention.StepID != EnsureStepLeapCLIAuth {
		t.Fatalf("expected warning to be attention check, got %q", attention.StepID)
	}
}

func findCheckByStep(checks []VerifiedCheck, stepID EnsureStepID) (VerifiedCheck, bool) {
	for _, check := range checks {
		if check.StepID == stepID {
			return check, true
		}
	}
	return VerifiedCheck{}, false
}
