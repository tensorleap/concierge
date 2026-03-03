package inspect

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelContract(repoRoot string, contract *leapYAMLContract, selectedModelPath string, status *core.IntegrationStatus) error {
	if contract == nil || status == nil {
		return nil
	}

	candidates, err := discoverModelCandidates(repoRoot, contract, status.Contracts)
	if err != nil {
		return err
	}
	attachModelCandidates(status, candidates)

	if len(candidates) == 0 {
		appendModelIssue(status, core.IssueCodeModelFileMissing, "no model candidate found; implement @tensorleap_load_model with a supported .onnx or .h5 artifact path", core.SeverityError)
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

	preferredCandidates := preferredModelEvaluations(evaluations)
	resolvable := resolvableModelEvaluations(preferredCandidates)

	selected := normalizeSelectedModelPath(selectedModelPath)
	if selected != "" {
		candidate, found := findModelEvaluationByDisplayPath(evaluations, selected)
		if !found {
			appendModelIssue(
				status,
				core.IssueCodeModelFileMissing,
				fmt.Sprintf("selected model path %q was not found among discovered candidates", selected),
				core.SeverityError,
			)
			return nil
		}
		if !candidate.SupportedFormat {
			appendModelIssue(
				status,
				core.IssueCodeModelFormatUnsupported,
				fmt.Sprintf("model candidate %q has unsupported format; expected .onnx or .h5", candidate.DisplayPath),
				core.SeverityError,
			)
			return nil
		}
		if !candidate.InsideRepo {
			appendModelIssue(
				status,
				core.IssueCodeModelFileMissing,
				fmt.Sprintf("model candidate %q points outside repository", candidate.Candidate.Path),
				core.SeverityError,
			)
			return nil
		}
		if !candidate.Exists {
			appendModelIssue(
				status,
				core.IssueCodeModelFileMissing,
				fmt.Sprintf("model file %q was not found", candidate.DisplayPath),
				core.SeverityError,
			)
			return nil
		}
		setResolvedModelPath(status, []modelCandidateEvaluation{candidate})
		return nil
	}

	if shouldDeferModelIssuesUntilPreprocess(status) {
		setResolvedModelPath(status, resolvable)
		return nil
	}

	if candidate, ok := firstUnsupportedModelCandidate(preferredCandidates); ok {
		appendModelIssue(
			status,
			core.IssueCodeModelFormatUnsupported,
			fmt.Sprintf("model candidate %q has unsupported format; expected .onnx or .h5", candidate.DisplayPath),
			core.SeverityError,
		)
	}

	if candidate, ok := firstOutsideRepoModelCandidate(preferredCandidates); ok {
		appendModelIssue(
			status,
			core.IssueCodeModelFileMissing,
			fmt.Sprintf("model candidate %q points outside repository", candidate.Candidate.Path),
			core.SeverityError,
		)
	}

	setResolvedModelPath(status, resolvable)

	if len(resolvable) > 1 {
		appendModelIssue(
			status,
			core.IssueCodeModelCandidatesAmbiguous,
			fmt.Sprintf(
				"multiple resolvable model candidates found: %s; make @tensorleap_load_model resolve a single .onnx/.h5 artifact",
				joinModelCandidatePaths(resolvable),
			),
			core.SeverityError,
		)
		return nil
	}

	if len(resolvable) == 0 {
		if candidate, ok := firstMissingModelFileCandidate(preferredCandidates); ok {
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
			fmt.Sprintf("no resolvable model candidate found; discovered candidates: %s", joinModelCandidatePaths(preferredCandidates)),
			core.SeverityError,
		)
	}

	return nil
}

func shouldDeferModelIssuesUntilPreprocess(status *core.IntegrationStatus) bool {
	if status == nil || status.Contracts == nil {
		return false
	}
	if len(status.Contracts.LoadModelFunctions) > 0 {
		return false
	}
	for _, issue := range status.Issues {
		if issue.Code == core.IssueCodePreprocessFunctionMissing {
			return true
		}
	}
	return false
}

func normalizeSelectedModelPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
}

func findModelEvaluationByDisplayPath(
	evaluations []modelCandidateEvaluation,
	selectedPath string,
) (modelCandidateEvaluation, bool) {
	target := strings.ToLower(strings.TrimSpace(selectedPath))
	if target == "" {
		return modelCandidateEvaluation{}, false
	}
	for _, evaluation := range evaluations {
		candidatePath := strings.ToLower(strings.TrimSpace(evaluation.DisplayPath))
		if candidatePath == target {
			return evaluation, true
		}
	}
	return modelCandidateEvaluation{}, false
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
		if isRepoSearchOnlySource(evaluation.Candidate.Source) {
			continue
		}
		explicit = append(explicit, evaluation)
	}
	return explicit
}

func preferredModelEvaluations(evaluations []modelCandidateEvaluation) []modelCandidateEvaluation {
	explicit := explicitModelEvaluations(evaluations)
	if len(explicit) > 0 {
		return explicit
	}
	return evaluations
}

func isRepoSearchOnlySource(source string) bool {
	parts := strings.Split(strings.TrimSpace(source), ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if trimmed != modelCandidateSourceRepoSearch {
			return false
		}
	}
	return true
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
