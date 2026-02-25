package core

import "testing"

func TestKnownIssueCodesSeededAndUnique(t *testing.T) {
	codes := KnownIssueCodes()
	if len(codes) < 70 {
		t.Fatalf("expected a broad seeded code catalog, got only %d codes", len(codes))
	}

	seen := make(map[IssueCode]struct{}, len(codes))
	for _, code := range codes {
		if _, exists := seen[code]; exists {
			t.Fatalf("duplicate issue code in catalog: %q", code)
		}
		seen[code] = struct{}{}
	}

	required := []IssueCode{
		IssueCodeLeapYAMLMissing,
		IssueCodePreprocessFunctionMissing,
		IssueCodeInputEncoderMissing,
		IssueCodeGTEncoderMissing,
		IssueCodeIntegrationTestMissing,
		IssueCodeModelFormatUnsupported,
		IssueCodeLeapServerUnreachable,
		IssueCodeUploadFailed,
	}

	for _, code := range required {
		if !IsKnownIssueCode(code) {
			t.Fatalf("expected required seeded code to exist: %q", code)
		}
	}
}

func TestKnownIssueCodesReturnsCopy(t *testing.T) {
	codes := KnownIssueCodes()
	codes[0] = IssueCode("mutated")

	fresh := KnownIssueCodes()
	if fresh[0] == IssueCode("mutated") {
		t.Fatal("expected KnownIssueCodes to return a defensive copy")
	}
}

func TestUnknownIssueCodesRemainValidInputs(t *testing.T) {
	unknown := IssueCode("external_adapter_specific_issue")
	if IsKnownIssueCode(unknown) {
		t.Fatalf("did not expect unknown code %q to be considered known", unknown)
	}

	issue := Issue{
		Code:     unknown,
		Message:  "adapter-specific issue",
		Severity: SeverityWarning,
		Scope:    IssueScopeValidation,
	}
	if issue.Code != unknown {
		t.Fatalf("expected issue to preserve unknown code, got: %q", issue.Code)
	}
}
