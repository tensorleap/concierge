package inspect

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelAcquisition(repoRoot string, contract *leapYAMLContract, selectedModelPath string, status *core.IntegrationStatus) error {
	if contract == nil || status == nil || status.Contracts == nil {
		return nil
	}
	if strings.TrimSpace(status.Contracts.EntryFile) == "" {
		return nil
	}

	readyArtifacts, err := discoverModelCandidates(repoRoot, contract, status.Contracts)
	if err != nil {
		return err
	}
	passiveLeads, err := discoverPassiveModelLeads(repoRoot)
	if err != nil {
		return err
	}

	status.Contracts.ModelCandidates = append([]core.ModelCandidate(nil), readyArtifacts...)
	status.Contracts.ModelAcquisition = &core.ModelAcquisitionArtifacts{
		ReadyArtifacts: append([]core.ModelCandidate(nil), readyArtifacts...),
		PassiveLeads:   append([]core.ModelCandidate(nil), passiveLeads...),
	}

	selectedEvaluation, selectedProvided, err := evaluateSelectedModelPath(repoRoot, selectedModelPath)
	if err != nil {
		return err
	}
	if selectedProvided && selectedEvaluation.SupportedFormat && selectedEvaluation.InsideRepo && selectedEvaluation.Exists {
		status.Contracts.ResolvedModelPath = strings.TrimSpace(selectedEvaluation.DisplayPath)
		return nil
	}

	readyEvaluations, err := evaluateModelCandidates(repoRoot, readyArtifacts)
	if err != nil {
		return err
	}
	if resolved, ok := defaultResolvableModelCandidate(readyEvaluations); ok {
		status.Contracts.ResolvedModelPath = strings.TrimSpace(resolved.DisplayPath)
		return nil
	}
	status.Contracts.ResolvedModelPath = ""

	if selectedProvided {
		switch {
		case !selectedEvaluation.SupportedFormat:
			appendModelIssue(
				status,
				core.IssueCodeModelFormatUnsupported,
				fmt.Sprintf("selected model output path %q is not a supported .onnx or .h5 artifact", strings.TrimSpace(selectedEvaluation.Candidate.Path)),
				core.SeverityError,
			)
			return nil
		case !selectedEvaluation.InsideRepo:
			appendModelIssue(
				status,
				core.IssueCodeModelAcquisitionUnresolved,
				fmt.Sprintf("selected model output path %q points outside the repository", strings.TrimSpace(selectedEvaluation.Candidate.Path)),
				core.SeverityError,
			)
			return nil
		case !selectedEvaluation.Exists:
			appendModelIssue(
				status,
				core.IssueCodeModelMaterializationOutputMissing,
				fmt.Sprintf("expected materialized model artifact %q was not found", strings.TrimSpace(selectedEvaluation.DisplayPath)),
				core.SeverityError,
			)
			return nil
		}
	}

	if len(passiveLeads) > 0 {
		appendModelIssue(
			status,
			core.IssueCodeModelAcquisitionRequired,
			fmt.Sprintf(
				"no ready .onnx/.h5 model artifact was found; model-like files were discovered: %s. Concierge needs to materialize one supported artifact before wiring @tensorleap_load_model",
				joinRawModelCandidatePaths(passiveLeads),
			),
			core.SeverityError,
		)
		return nil
	}

	appendModelIssue(
		status,
		core.IssueCodeModelAcquisitionUnresolved,
		"no ready .onnx/.h5 model artifact was found and no model-like files were discovered. Concierge needs existing repository download/export logic or manual instructions to materialize a supported artifact",
		core.SeverityError,
	)
	return nil
}

func discoverPassiveModelLeads(repoRoot string) ([]core.ModelCandidate, error) {
	leads := make([]core.ModelCandidate, 0, 4)
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, ok := modelLikeExtensions[ext]; !ok {
			return nil
		}
		if isSupportedModelExtension(ext) {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		leads = append(leads, core.ModelCandidate{
			Path:   filepath.ToSlash(filepath.Clean(rel)),
			Source: "repo_scan.passive_lead",
		})
		return nil
	})
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.model_acquisition.passive_leads", err)
	}
	return uniqueSortedModelCandidates(leads), nil
}

func evaluateModelCandidates(repoRoot string, candidates []core.ModelCandidate) ([]modelCandidateEvaluation, error) {
	evaluations := make([]modelCandidateEvaluation, 0, len(candidates))
	for _, candidate := range candidates {
		evaluation, err := evaluateModelCandidate(repoRoot, candidate)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(evaluation.DisplayPath) == "" {
			continue
		}
		evaluations = append(evaluations, evaluation)
	}
	return evaluations, nil
}

func defaultResolvableModelCandidate(evaluations []modelCandidateEvaluation) (modelCandidateEvaluation, bool) {
	for _, evaluation := range evaluations {
		if !evaluation.SupportedFormat || !evaluation.InsideRepo || !evaluation.Exists {
			continue
		}
		return evaluation, true
	}
	return modelCandidateEvaluation{}, false
}

func evaluateSelectedModelPath(repoRoot, selectedModelPath string) (modelCandidateEvaluation, bool, error) {
	normalized := normalizeSelectedModelPath(selectedModelPath)
	if normalized == "" {
		return modelCandidateEvaluation{}, false, nil
	}
	evaluation, err := evaluateModelCandidate(repoRoot, core.ModelCandidate{
		Path:   normalized,
		Source: "selected_model_path",
	})
	if err != nil {
		return modelCandidateEvaluation{}, true, err
	}
	return evaluation, true, nil
}

func uniqueSortedModelCandidates(candidates []core.ModelCandidate) []core.ModelCandidate {
	if len(candidates) == 0 {
		return nil
	}
	byKey := make(map[string]core.ModelCandidate, len(candidates))
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			continue
		}
		key := strings.ToLower(path)
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = core.ModelCandidate{
			Path:   path,
			Source: strings.TrimSpace(candidate.Source),
		}
	}
	if len(byKey) == 0 {
		return nil
	}
	values := make([]core.ModelCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		values = append(values, candidate)
	}
	sortModelCandidates(values)
	return values
}

func sortModelCandidates(candidates []core.ModelCandidate) {
	if len(candidates) < 2 {
		return
	}
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			left := strings.ToLower(strings.TrimSpace(candidates[i].Path))
			right := strings.ToLower(strings.TrimSpace(candidates[j].Path))
			if left < right {
				continue
			}
			if left == right && candidates[i].Source <= candidates[j].Source {
				continue
			}
			candidates[i], candidates[j] = candidates[j], candidates[i]
		}
	}
}

func joinRawModelCandidatePaths(candidates []core.ModelCandidate) string {
	if len(candidates) == 0 {
		return "none"
	}
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if path := strings.TrimSpace(candidate.Path); path != "" {
			paths = append(paths, path)
		}
	}
	paths = uniqueSortedStrings(paths)
	if len(paths) == 0 {
		return "none"
	}
	return strings.Join(paths, ", ")
}
