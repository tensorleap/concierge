package execute

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// BuildGTEncoderAuthoringRecommendation builds deterministic remediation guidance for ensure.ground_truth_encoders.
func BuildGTEncoderAuthoringRecommendation(
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
) (core.AuthoringRecommendation, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AuthoringRecommendation{}, core.NewError(
			core.KindUnknown,
			"execute.gt_encoder_authoring.repo_root",
			"snapshot repository root is empty",
		)
	}

	missingSymbols := collectGTEncoderRecommendationSymbols(repoRoot, status)
	constraints := []string{
		"Implement @tensorleap_gt_encoder functions for each missing target symbol.",
		"Ground-truth encoders must run on labeled subsets only (never unlabeled subsets).",
		"Do not modify @tensorleap_input_encoder definitions in this step.",
		"After adding each encoder, wire it into the @tensorleap_integration_test function body so the code_loader status table sees it exercised.",
	}
	if selectedModelPath := strings.TrimSpace(snapshot.SelectedModelPath); selectedModelPath != "" {
		constraints = append(constraints, fmt.Sprintf("Use model path %q as the output/label alignment reference unless repository evidence proves it invalid.", selectedModelPath))
	}

	recommendation := core.AuthoringRecommendation{
		StepID:      core.EnsureStepGroundTruthEncoders,
		Candidates:  missingSymbols,
		Constraints: constraints,
	}

	if len(missingSymbols) > 0 {
		recommendation.Target = missingSymbols[0]
		recommendation.Rationale = "missing ground-truth symbols discovered from integration contracts"
		return recommendation, nil
	}

	recommendation.Rationale = "add or repair @tensorleap_gt_encoder mappings for labeled-subset targets"
	return recommendation, nil
}

func collectGTEncoderRecommendationSymbols(repoRoot string, status core.IntegrationStatus) []string {
	symbolsFromIssues := recommendationSymbolsFromIssues(status.Issues,
		core.IssueCodeGTEncoderMissing,
		core.IssueCodeGTEncoderCoverageIncomplete,
		core.IssueCodeUnlabeledSubsetGTInvocation,
	)
	if len(symbolsFromIssues) > 0 {
		return symbolsFromIssues
	}

	source, ok := resolveAuthoringEntrySource(repoRoot, status.Contracts)
	if !ok {
		return nil
	}

	return deriveMissingGTSymbolsFromSource(source, status.Contracts)
}

func deriveMissingGTSymbolsFromSource(source string, contracts *core.IntegrationContracts) []string {
	gtRegistrations := discoverAuthoringEncoderRegistrations(source, "tensorleap_gt_encoder")
	inputRegistrations := discoverAuthoringEncoderRegistrations(source, "tensorleap_input_encoder")
	integrationCalls := discoverAuthoringIntegrationCalls(source, contracts)

	registrationByFunction := mapAuthoringRegistrationsByFunction(gtRegistrations)
	excludedFunctions := authoringFunctionSet(contracts, inputRegistrations, nil)

	expected := make([]string, 0, len(gtRegistrations))
	for _, registration := range gtRegistrations {
		expected = append(expected, registration.Symbol)
		if !registration.HasExplicitSymbol {
			expected = append(expected, registration.Symbol)
		}
	}

	for _, rawCall := range integrationCalls {
		call := canonicalAuthoringSymbol(rawCall)
		if call == "" {
			continue
		}
		key := strings.ToLower(call)
		if _, excluded := excludedFunctions[key]; excluded {
			continue
		}
		if registration, ok := registrationByFunction[key]; ok {
			expected = append(expected, registration.Symbol)
			continue
		}
		if shouldTreatAsGTAuthoringCall(call, contracts, inputRegistrations, gtRegistrations) {
			expected = append(expected, inferAuthoringSymbol(call))
		}
	}

	actual := make([]string, 0, len(gtRegistrations))
	for _, registration := range gtRegistrations {
		actual = append(actual, registration.Symbol)
	}

	missing := missingAuthoringSymbols(expected, actual)
	for _, registration := range gtRegistrations {
		if registration.HasExplicitSymbol {
			continue
		}
		missing = append(missing, registration.Symbol)
	}
	return uniqueSortedStrings(missing)
}

func shouldTreatAsGTAuthoringCall(
	call string,
	contracts *core.IntegrationContracts,
	inputRegistrations []authoringEncoderRegistration,
	gtRegistrations []authoringEncoderRegistration,
) bool {
	if looksLikeGTAuthoringCall(call) {
		return true
	}

	normalizedCall := normalizeAuthoringSymbol(call)
	if !strings.HasPrefix(normalizedCall, "encode_") {
		return false
	}

	for _, registration := range inputRegistrations {
		if normalizeAuthoringSymbol(registration.Function) == normalizedCall {
			return false
		}
	}

	if contracts != nil {
		for _, inputFunction := range contracts.InputEncoders {
			if normalizeAuthoringSymbol(inputFunction) == normalizedCall {
				return false
			}
		}
		if len(contracts.GroundTruthEncoders) > 0 {
			return true
		}
	}

	return len(gtRegistrations) > 0 && len(inputRegistrations) == 0
}
