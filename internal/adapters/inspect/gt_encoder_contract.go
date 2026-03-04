package inspect

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectGTEncoderContract(repoRoot string, status *core.IntegrationStatus) {
	if status == nil || status.Contracts == nil {
		return
	}

	entryFilePath, source, ok := loadContractEntrySource(repoRoot, status.Contracts)
	if !ok {
		return
	}

	registrations := discoverEncoderRegistrations(source, "tensorleap_gt_encoder")
	expected := expectedGTEncoderSymbols(status.Contracts, registrations)
	actual := encoderRegistrationSymbols(registrations)
	missing := missingContractSymbols(expected, actual)

	if len(missing) > 0 {
		issueCode := core.IssueCodeGTEncoderCoverageIncomplete
		template := "ground-truth encoder contract mismatch: missing required symbol %q"
		if len(actual) == 0 {
			issueCode = core.IssueCodeGTEncoderMissing
			template = "missing @tensorleap_gt_encoder for required target symbol %q"
		}

		for _, symbol := range missing {
			status.Issues = append(status.Issues, core.Issue{
				Code:     issueCode,
				Message:  fmt.Sprintf(template, symbol),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Symbol: symbol,
				},
			})
		}
	}

	for _, registration := range registrations {
		if !registration.HasExplicitSymbol {
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodeGTEncoderCoverageIncomplete,
				Message:  fmt.Sprintf("@tensorleap_gt_encoder function %q does not declare a target symbol; contract mapping is ambiguous", registration.Function),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Line:   registration.Line,
					Symbol: registration.Function,
				},
			})
		}

		if registration.AppliesToUnlabeled {
			target := strings.TrimSpace(registration.Symbol)
			if target == "" {
				target = strings.TrimSpace(registration.Function)
			}
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodeUnlabeledSubsetGTInvocation,
				Message:  fmt.Sprintf("@tensorleap_gt_encoder for %q is configured to run on unlabeled data; GT encoders must run on labeled subsets only", target),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: &core.IssueLocation{
					Path:   entryFilePath,
					Line:   registration.Line,
					Symbol: target,
				},
			})
		}
	}
}

func expectedGTEncoderSymbols(contracts *core.IntegrationContracts, registrations []encoderRegistration) []string {
	expected := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		expected = append(expected, registration.Symbol)
	}

	if contracts == nil {
		return uniqueSortedContractSymbols(expected)
	}

	registrationsByFunction := mapEncoderRegistrationsByFunction(registrations)
	excludedCalls := contractSymbolSet(
		contracts.LoadModelFunctions,
		contracts.PreprocessFunctions,
		contracts.InputEncoders,
		contracts.IntegrationTestFunctions,
	)

	for _, rawCall := range contracts.IntegrationTestCalls {
		call := strings.TrimSpace(canonicalSymbol(rawCall))
		if call == "" {
			continue
		}
		key := strings.ToLower(call)
		if _, excluded := excludedCalls[key]; excluded {
			continue
		}

		if registration, ok := registrationsByFunction[key]; ok {
			expected = append(expected, registration.Symbol)
			continue
		}

		if shouldTreatAsGroundTruthCall(call, contracts) {
			expected = append(expected, inferEncoderSymbol(call))
		}
	}

	return uniqueSortedContractSymbols(expected)
}

func shouldTreatAsGroundTruthCall(call string, contracts *core.IntegrationContracts) bool {
	if looksLikeGroundTruthEncoderCall(call) {
		return true
	}

	lower := strings.ToLower(strings.TrimSpace(call))
	if !strings.HasPrefix(lower, "encode_") {
		return false
	}

	if contracts == nil {
		return false
	}

	for _, inputFunction := range contracts.InputEncoders {
		if strings.EqualFold(strings.TrimSpace(inputFunction), lower) {
			return false
		}
	}

	return len(contracts.GroundTruthEncoders) > 0
}
