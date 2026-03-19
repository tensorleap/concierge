package inspect

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelAcquisition(
	snapshot core.WorkspaceSnapshot,
	repoRoot string,
	contract *leapYAMLContract,
	selectedModelPath string,
	status *core.IntegrationStatus,
) error {
	if contract == nil || status == nil || status.Contracts == nil {
		return nil
	}
	if strings.TrimSpace(status.Contracts.EntryFile) == "" {
		return nil
	}

	discoveredCandidates, err := discoverModelCandidates(repoRoot, contract, status.Contracts)
	if err != nil {
		return err
	}
	passiveLeads, err := discoverPassiveModelLeads(repoRoot)
	if err != nil {
		return err
	}
	acquisitionLeads, err := discoverModelAcquisitionLeads(repoRoot)
	if err != nil {
		return err
	}

	readyEvaluations, err := evaluateModelCandidates(repoRoot, discoveredCandidates)
	if err != nil {
		return err
	}
	readyArtifacts, verifiedArtifacts, failedArtifacts := annotateModelCandidates(snapshot, readyEvaluations)

	status.Contracts.ModelCandidates = append([]core.ModelCandidate(nil), readyArtifacts...)
	status.Contracts.ModelAcquisition = &core.ModelAcquisitionArtifacts{
		ReadyArtifacts:   append([]core.ModelCandidate(nil), readyArtifacts...),
		PassiveLeads:     append([]core.ModelCandidate(nil), passiveLeads...),
		AcquisitionLeads: append([]string(nil), acquisitionLeads...),
	}

	selectedEvaluation, selectedProvided, err := evaluateSelectedModelPath(repoRoot, selectedModelPath)
	if err != nil {
		return err
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

	if selectedProvided && selectedEvaluation.SupportedFormat && selectedEvaluation.InsideRepo && selectedEvaluation.Exists {
		if selectedCandidate, verified := verifySelectedModelCandidate(snapshot, selectedEvaluation); verified {
			status.Contracts.ResolvedModelPath = strings.TrimSpace(selectedCandidate.Path)
			return nil
		} else if runtimeVerificationAvailable(snapshot) {
			appendModelIssue(
				status,
				core.IssueCodeModelAcquisitionUnresolved,
				runtimeVerificationFailureMessage([]core.ModelCandidate{selectedCandidate}),
				core.SeverityError,
			)
			return nil
		}
	}

	if runtimeVerificationAvailable(snapshot) {
		switch len(verifiedArtifacts) {
		case 1:
			status.Contracts.ResolvedModelPath = strings.TrimSpace(verifiedArtifacts[0].Path)
			return nil
		case 0:
			if len(failedArtifacts) > 0 {
				appendModelIssue(
					status,
					core.IssueCodeModelAcquisitionUnresolved,
					runtimeVerificationFailureMessage(failedArtifacts),
					core.SeverityError,
				)
				return nil
			}
		default:
			appendModelIssue(
				status,
				core.IssueCodeModelCandidatesAmbiguous,
				ambiguousModelCandidatesMessage(verifiedArtifacts),
				core.SeverityError,
			)
			return nil
		}
	} else if hasExistingModelCandidate(readyArtifacts) {
		return nil
	}

	if len(missingModelCandidatePaths(readyArtifacts)) > 0 || len(passiveLeads) > 0 || len(acquisitionLeads) > 0 {
		appendModelIssue(
			status,
			core.IssueCodeModelAcquisitionRequired,
			modelAcquisitionIssueMessage(readyArtifacts, passiveLeads, acquisitionLeads),
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

func modelAcquisitionIssueMessage(readyArtifacts []core.ModelCandidate, passiveLeads []core.ModelCandidate, acquisitionLeads []string) string {
	values := make([]string, 0, len(readyArtifacts)+len(passiveLeads)+len(acquisitionLeads))
	for _, candidate := range readyArtifacts {
		if candidate.Exists {
			continue
		}
		if path := strings.TrimSpace(candidate.Path); path != "" {
			values = append(values, path)
		}
	}
	for _, candidate := range passiveLeads {
		if path := strings.TrimSpace(candidate.Path); path != "" {
			values = append(values, path)
		}
	}
	values = append(values, acquisitionLeads...)
	values = uniqueSortedStrings(values)
	if len(values) == 0 {
		return "no ready .onnx/.h5 model artifact was found and no model-like files were discovered. Concierge needs existing repository download/export logic or manual instructions to materialize a supported artifact"
	}
	return fmt.Sprintf(
		"no ready .onnx/.h5 model artifact was found; repository model acquisition leads were discovered: %s. Concierge needs to materialize one supported artifact before wiring @tensorleap_load_model",
		strings.Join(values, ", "),
	)
}

func discoverPassiveModelLeads(repoRoot string) ([]core.ModelCandidate, error) {
	leads := make([]core.ModelCandidate, 0, 4)
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipModelScanDir(entry.Name()) {
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

func annotateModelCandidates(
	snapshot core.WorkspaceSnapshot,
	evaluations []modelCandidateEvaluation,
) ([]core.ModelCandidate, []core.ModelCandidate, []core.ModelCandidate) {
	annotated := make([]core.ModelCandidate, 0, len(evaluations))
	verified := make([]core.ModelCandidate, 0, len(evaluations))
	failed := make([]core.ModelCandidate, 0, len(evaluations))
	for _, evaluation := range evaluations {
		candidate, isVerified := annotateModelCandidate(snapshot, evaluation)
		if strings.TrimSpace(candidate.Path) == "" {
			continue
		}
		annotated = append(annotated, candidate)
		if isVerified {
			verified = append(verified, candidate)
			continue
		}
		if candidate.VerificationState == core.ModelCandidateVerificationStateFailed {
			failed = append(failed, candidate)
		}
	}
	return annotated, verified, failed
}

func annotateModelCandidate(snapshot core.WorkspaceSnapshot, evaluation modelCandidateEvaluation) (core.ModelCandidate, bool) {
	candidate := core.ModelCandidate{
		Path:   strings.TrimSpace(evaluation.DisplayPath),
		Source: strings.TrimSpace(evaluation.Candidate.Source),
		Exists: evaluation.Exists,
	}
	if !evaluation.Exists || !evaluation.SupportedFormat || !evaluation.InsideRepo {
		return candidate, false
	}
	if !runtimeVerificationAvailable(snapshot) {
		candidate.VerificationState = core.ModelCandidateVerificationStateUnverified
		return candidate, false
	}
	if err := verifyModelCandidateInRuntime(snapshot, evaluation); err == nil {
		candidate.VerificationState = core.ModelCandidateVerificationStateVerified
		return candidate, true
	} else {
		candidate.VerificationState = core.ModelCandidateVerificationStateFailed
		candidate.VerificationError = strings.TrimSpace(err.Error())
	}
	return candidate, false
}

func runtimeVerificationAvailable(snapshot core.WorkspaceSnapshot) bool {
	return snapshot.RuntimeProfile != nil && strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath) != ""
}

func verifySelectedModelCandidate(snapshot core.WorkspaceSnapshot, evaluation modelCandidateEvaluation) (core.ModelCandidate, bool) {
	candidate, verified := annotateModelCandidate(snapshot, evaluation)
	return candidate, verified
}

func verifyModelCandidateInRuntime(snapshot core.WorkspaceSnapshot, evaluation modelCandidateEvaluation) error {
	modelType := runtimeVerificationModelType(evaluation)
	if strings.TrimSpace(modelType) == "" {
		return fmt.Errorf("unsupported model type")
	}
	_, err := runtimeSignatureProbeRunner(snapshot, evaluation.AbsolutePath, modelType)
	return err
}

func runtimeVerificationModelType(evaluation modelCandidateEvaluation) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(evaluation.DisplayPath))) {
	case ".onnx":
		return "onnx"
	case ".h5", ".keras":
		return "keras"
	default:
		return ""
	}
}

func ambiguousModelCandidatesMessage(candidates []core.ModelCandidate) string {
	return fmt.Sprintf(
		"multiple supported model artifacts were verified in the prepared runtime: %s. Concierge needs the user to choose which model source to follow",
		joinRawModelCandidatePaths(candidates),
	)
}

func runtimeVerificationFailureMessage(candidates []core.ModelCandidate) string {
	failures := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			continue
		}
		detail := path
		if message := strings.TrimSpace(candidate.VerificationError); message != "" {
			detail = fmt.Sprintf("%s (%s)", path, message)
		}
		failures = append(failures, detail)
	}
	failures = uniqueSortedStrings(failures)
	return fmt.Sprintf(
		"supported model artifacts were found but could not be loaded in the prepared runtime: %s",
		strings.Join(failures, ", "),
	)
}

func hasExistingModelCandidate(candidates []core.ModelCandidate) bool {
	for _, candidate := range candidates {
		if candidate.Exists {
			return true
		}
	}
	return false
}

func missingModelCandidatePaths(candidates []core.ModelCandidate) []string {
	values := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Exists {
			continue
		}
		if path := strings.TrimSpace(candidate.Path); path != "" {
			values = append(values, path)
		}
	}
	return uniqueSortedStrings(values)
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
