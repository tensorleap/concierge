package fixtures

import (
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
)

func TestUltralyticsPostDoesNotReportBinderHostedContractGaps(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	_, postRoot := resolveFixtureRoots(t, "ultralytics")
	status := inspectStatus(t, postRoot)

	if containsIssueCode(status.Issues, core.IssueCodePreprocessFunctionMissing) {
		t.Fatalf(
			"did not expect %q for ultralytics post fixture, contracts=%+v issues=%+v",
			core.IssueCodePreprocessFunctionMissing,
			status.Contracts,
			status.Issues,
		)
	}
	if containsAnyIssueCode(status.Issues, core.IssueCodeInputEncoderMissing, core.IssueCodeInputEncoderCoverageIncomplete) {
		t.Fatalf("did not expect input-encoder coverage gaps for ultralytics post fixture, contracts=%+v issues=%+v", status.Contracts, status.Issues)
	}
}
