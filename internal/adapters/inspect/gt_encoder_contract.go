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

	sources := loadContractSources(repoRoot, status.Contracts)
	if len(sources) == 0 {
		return
	}

	registrations := make([]encoderRegistration, 0, 8)
	for _, source := range sources {
		registrations = append(registrations, discoverEncoderRegistrations(source.Contents, "tensorleap_gt_encoder")...)
	}
	expected := expectedGTEncoderSymbols(status.Contracts, registrations)
	actual := encoderRegistrationSymbols(registrations)
	missing := missingContractSymbols(expected, actual)
	issuePath := primaryContractSourcePath(sources, registrations)

	if len(missing) > 0 {
		issueCode := core.IssueCodeGTEncoderCoverageIncomplete
		template := "ground-truth encoder coverage is incomplete: missing required ground-truth name %q"
		if len(actual) == 0 {
			issueCode = core.IssueCodeGTEncoderMissing
			template = "missing @tensorleap_gt_encoder for required ground-truth name %q"
		}

		for _, symbol := range missing {
			status.Issues = append(status.Issues, core.Issue{
				Code:     issueCode,
				Message:  fmt.Sprintf(template, symbol),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: &core.IssueLocation{
					Path:   issuePath,
					Symbol: symbol,
				},
			})
		}
	}

	for _, registration := range registrations {
		if !registration.HasExplicitSymbol {
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodeGTEncoderCoverageIncomplete,
				Message:  fmt.Sprintf("@tensorleap_gt_encoder function %q does not declare a ground-truth name; contract mapping is ambiguous", registration.Function),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeGroundTruthEncoder,
				Location: &core.IssueLocation{
					Path:   issuePath,
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
					Path:   issuePath,
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
	if contracts.ConfirmedMapping != nil && len(contracts.ConfirmedMapping.GroundTruthSymbols) > 0 {
		expected = append(expected, contracts.ConfirmedMapping.GroundTruthSymbols...)
		return uniqueSortedContractSymbols(expected)
	}
	// Discovery-derived symbols are intentionally only enforced when no GT encoder
	// registrations exist yet; once at least one encoder exists, confirmation is
	// required to avoid noisy false positives from alternate semantic branches.
	if len(expected) == 0 && len(contracts.DiscoveredGroundTruthSymbols) > 0 {
		expected = append(expected, contracts.DiscoveredGroundTruthSymbols...)
		return uniqueSortedContractSymbols(expected)
	}

	return uniqueSortedContractSymbols(expected)
}
