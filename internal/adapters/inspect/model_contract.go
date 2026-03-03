package inspect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelContract(repoRoot string, contract *leapYAMLContract, status *core.IntegrationStatus) error {
	if contract == nil || status == nil {
		return nil
	}

	candidates, err := discoverModelCandidates(repoRoot, contract, status.Contracts)
	if err != nil {
		return err
	}
	attachModelCandidates(status, candidates)

	if len(candidates) == 0 {
		appendModelIssue(status, core.IssueCodeModelFileMissing, "no model candidate found; set leap.yaml modelPath/model or provide a discoverable .onnx/.h5 model", core.SeverityError)
		return nil
	}

	evaluations := make([]modelCandidateEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		evaluation, evalErr := evaluateModelCandidate(repoRoot, candidate)
		if evalErr != nil {
			return evalErr
		}
		if strings.TrimSpace(evaluation.DisplayPath) == "" {
			continue
		}
		evaluations = append(evaluations, evaluation)
	}

	issueCandidates := explicitModelEvaluations(evaluations)
	if len(issueCandidates) == 0 {
		issueCandidates = evaluations
	}

	if candidate, ok := firstUnsupportedModelCandidate(issueCandidates); ok {
		appendModelIssue(
			status,
			core.IssueCodeModelFormatUnsupported,
			fmt.Sprintf("model candidate %q has unsupported format; expected .onnx or .h5", candidate.DisplayPath),
			core.SeverityError,
		)
	}

	if candidate, ok := firstOutsideRepoModelCandidate(issueCandidates); ok {
		appendModelIssue(
			status,
			core.IssueCodeModelFileMissing,
			fmt.Sprintf("model candidate %q points outside repository", candidate.Candidate.Path),
			core.SeverityError,
		)
	}

	resolvable := resolvableModelEvaluations(evaluations)
	setResolvedModelPath(status, resolvable)

	if len(resolvable) > 1 {
		appendModelIssue(
			status,
			core.IssueCodeModelCandidatesAmbiguous,
			fmt.Sprintf(
				"multiple resolvable model candidates found: %s; set leap.yaml modelPath/model to disambiguate",
				joinModelCandidatePaths(resolvable),
			),
			core.SeverityError,
		)
		return nil
	}

	if len(resolvable) == 0 {
		if candidate, ok := firstMissingModelFileCandidate(issueCandidates); ok {
			appendModelIssue(
				status,
				core.IssueCodeModelFileMissing,
				fmt.Sprintf("model file %q was not found", candidate.DisplayPath),
				core.SeverityError,
			)
			return nil
		}
		appendModelIssue(
			status,
			core.IssueCodeModelFileMissing,
			fmt.Sprintf("no resolvable model candidate found; discovered candidates: %s", joinModelCandidatePaths(evaluations)),
			core.SeverityError,
		)
	}

	return nil
}

func attachModelCandidates(status *core.IntegrationStatus, candidates []core.ModelCandidate) {
	if status == nil || status.Contracts == nil {
		return
	}
	if len(candidates) == 0 {
		status.Contracts.ModelCandidates = nil
		status.Contracts.ResolvedModelPath = ""
		return
	}
	status.Contracts.ModelCandidates = append([]core.ModelCandidate(nil), candidates...)
	status.Contracts.ResolvedModelPath = ""
}

func setResolvedModelPath(status *core.IntegrationStatus, resolvable []modelCandidateEvaluation) {
	if status == nil || status.Contracts == nil {
		return
	}
	if len(resolvable) != 1 {
		status.Contracts.ResolvedModelPath = ""
		return
	}
	status.Contracts.ResolvedModelPath = strings.TrimSpace(resolvable[0].DisplayPath)
}

func explicitModelEvaluations(evaluations []modelCandidateEvaluation) []modelCandidateEvaluation {
	explicit := make([]modelCandidateEvaluation, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if strings.Contains(evaluation.Candidate.Source, modelCandidateSourceRepoSearch) {
			continue
		}
		explicit = append(explicit, evaluation)
	}
	return explicit
}

func firstUnsupportedModelCandidate(evaluations []modelCandidateEvaluation) (modelCandidateEvaluation, bool) {
	for _, evaluation := range evaluations {
		if evaluation.SupportedFormat {
			continue
		}
		return evaluation, true
	}
	return modelCandidateEvaluation{}, false
}

func firstOutsideRepoModelCandidate(evaluations []modelCandidateEvaluation) (modelCandidateEvaluation, bool) {
	for _, evaluation := range evaluations {
		if !evaluation.InsideRepo {
			return evaluation, true
		}
	}
	return modelCandidateEvaluation{}, false
}

func firstMissingModelFileCandidate(evaluations []modelCandidateEvaluation) (modelCandidateEvaluation, bool) {
	for _, evaluation := range evaluations {
		if !evaluation.InsideRepo || !evaluation.SupportedFormat {
			continue
		}
		if evaluation.Exists {
			continue
		}
		return evaluation, true
	}
	return modelCandidateEvaluation{}, false
}

func resolvableModelEvaluations(evaluations []modelCandidateEvaluation) []modelCandidateEvaluation {
	resolvable := make([]modelCandidateEvaluation, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if !evaluation.SupportedFormat {
			continue
		}
		if !evaluation.InsideRepo {
			continue
		}
		if !evaluation.Exists {
			continue
		}
		resolvable = append(resolvable, evaluation)
	}
	return resolvable
}

func appendModelIssue(status *core.IntegrationStatus, code core.IssueCode, message string, severity core.Severity) {
	if status == nil {
		return
	}
	status.Issues = append(status.Issues, core.Issue{
		Code:     code,
		Message:  message,
		Severity: severity,
		Scope:    core.IssueScopeModel,
	})
}

func joinModelCandidatePaths(candidates []modelCandidateEvaluation) string {
	if len(candidates) == 0 {
		return "none"
	}

	paths := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate.DisplayPath)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return "none"
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}
