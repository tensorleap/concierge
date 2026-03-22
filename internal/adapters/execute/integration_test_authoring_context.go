package execute

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// BuildIntegrationTestAuthoringRecommendation builds deterministic remediation guidance
// for ensure.integration_test_contract.
func BuildIntegrationTestAuthoringRecommendation(
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
) (core.AuthoringRecommendation, error) {
	recommendation := core.AuthoringRecommendation{
		StepID: core.EnsureStepIntegrationTestWiring,
		Constraints: []string{
			"Repair only @tensorleap_integration_test wiring and keep the body thin and declarative.",
			"Do not modify preprocess subset semantics, encoder implementations, or unrelated training/business logic in this step.",
		},
	}

	missingCalls := integrationTestIssueTargets(status.Issues,
		core.IssueCodeIntegrationTestMissingRequiredCalls,
	)
	illegalBodyLogic := integrationTestIssueTargets(status.Issues,
		core.IssueCodeIntegrationTestCallsUnknownInterfaces,
		core.IssueCodeIntegrationTestDirectDatasetAccess,
		core.IssueCodeIntegrationTestIllegalBodyLogic,
		core.IssueCodeIntegrationTestManualBatchManipulation,
	)

	if len(missingCalls) > 0 {
		recommendation.Target = missingCalls[0]
		recommendation.Rationale = "repair missing required decorator calls inside @tensorleap_integration_test"
		recommendation.Candidates = append(recommendation.Candidates, missingCalls...)
		recommendation.Constraints = append(
			recommendation.Constraints,
			fmt.Sprintf("First repair missing decorated calls: %s.", strings.Join(missingCalls, ", ")),
		)
	}
	if len(illegalBodyLogic) > 0 {
		if recommendation.Target == "" {
			recommendation.Target = illegalBodyLogic[0]
			recommendation.Rationale = "remove illegal integration-test body logic so mapping-mode re-execution can succeed"
		}
		recommendation.Candidates = append(recommendation.Candidates, illegalBodyLogic...)
		recommendation.Constraints = append(
			recommendation.Constraints,
			fmt.Sprintf("Then remove illegal body logic: %s.", strings.Join(illegalBodyLogic, ", ")),
		)
	}
	if recommendation.Rationale == "" {
		recommendation.Rationale = "repair @tensorleap_integration_test wiring so required decorators are called without plain Python logic"
	}

	recommendation.Candidates = uniqueSortedStrings(recommendation.Candidates)
	return recommendation, nil
}

func integrationTestIssueTargets(issues []core.Issue, codes ...core.IssueCode) []string {
	if len(issues) == 0 || len(codes) == 0 {
		return nil
	}

	codeSet := make(map[core.IssueCode]struct{}, len(codes))
	for _, code := range codes {
		codeSet[code] = struct{}{}
	}

	targets := make([]string, 0, len(issues))
	for _, issue := range issues {
		if _, ok := codeSet[issue.Code]; !ok {
			continue
		}
		target := integrationTestIssueTarget(issue)
		if target == "" {
			continue
		}
		targets = append(targets, target)
	}
	return uniqueSortedStrings(targets)
}

func integrationTestIssueTarget(issue core.Issue) string {
	if issue.Location != nil {
		if symbol := strings.TrimSpace(issue.Location.Symbol); symbol != "" {
			return strings.ToLower(symbol)
		}
	}

	message := strings.ToLower(strings.TrimSpace(issue.Message))
	switch issue.Code {
	case core.IssueCodeIntegrationTestManualBatchManipulation:
		return "manual_batching"
	case core.IssueCodeIntegrationTestDirectDatasetAccess:
		return "dataset_access"
	case core.IssueCodeIntegrationTestIllegalBodyLogic:
		if strings.Contains(message, "indexing is only allowed") {
			return "non_prediction_indexing"
		}
		return "body_logic"
	case core.IssueCodeIntegrationTestCallsUnknownInterfaces:
		return "non_decorated_calls"
	default:
		return ""
	}
}
