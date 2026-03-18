package execute

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var supportedModelAuthoringExtensions = map[string]struct{}{
	".onnx": {},
	".h5":   {},
}

type modelRecommendationCandidate struct {
	Path         string
	Supported    bool
	InsideRepo   bool
	SourceWeight int
}

// BuildModelAuthoringRecommendation builds deterministic model-remediation guidance for ensure.model_contract.
func BuildModelAuthoringRecommendation(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) (core.AuthoringRecommendation, error) {
	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.AuthoringRecommendation{}, core.NewError(
			core.KindUnknown,
			"execute.model_authoring.repo_root",
			"snapshot repository root is empty",
		)
	}

	candidates, err := collectModelRecommendationCandidates(repoRoot, snapshot, status)
	if err != nil {
		return core.AuthoringRecommendation{}, err
	}

	target, rationale := selectModelRecommendationTarget(repoRoot, snapshot, status, candidates)
	candidatePaths := orderedCandidatePaths(candidates)
	if target != "" {
		candidatePaths = ensureValuePresent(candidatePaths, target)
	}
	candidatePaths = truncateCandidatePaths(candidatePaths, target, maxRepoContextModelCandidates)

	constraints := []string{
		"Bind @tensorleap_load_model to one concrete supported model artifact path.",
		"Model artifact path must end with .onnx or .h5.",
		"Model binaries are uploaded by leap CLI; leap.yaml include/exclude governs integration code.",
		"Do not modify unrelated training/business logic.",
	}

	return core.AuthoringRecommendation{
		StepID:      core.EnsureStepModelContract,
		Target:      target,
		Rationale:   rationale,
		Candidates:  candidatePaths,
		Constraints: constraints,
	}, nil
}

func collectModelRecommendationCandidates(
	repoRoot string,
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
) ([]modelRecommendationCandidate, error) {
	byKey := map[string]modelRecommendationCandidate{}
	add := func(raw string, sourceWeight int) {
		candidate := evaluateModelRecommendationCandidate(repoRoot, raw, sourceWeight)
		if candidate.Path == "" {
			return
		}
		key := strings.ToLower(candidate.Path)
		existing, exists := byKey[key]
		if !exists || candidate.SourceWeight < existing.SourceWeight {
			byKey[key] = candidate
		}
	}

	add(snapshot.SelectedModelPath, 0)
	if status.Contracts != nil {
		add(status.Contracts.ResolvedModelPath, 1)
		for _, candidate := range status.Contracts.ModelCandidates {
			add(candidate.Path, 2)
		}
	}

	candidates := make([]modelRecommendationCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := strings.ToLower(candidates[i].Path)
		right := strings.ToLower(candidates[j].Path)
		if left != right {
			return left < right
		}
		return candidates[i].Path < candidates[j].Path
	})

	return candidates, nil
}

func selectModelRecommendationTarget(
	repoRoot string,
	snapshot core.WorkspaceSnapshot,
	status core.IntegrationStatus,
	candidates []modelRecommendationCandidate,
) (string, string) {
	if selected := normalizeModelRecommendationPath(repoRoot, snapshot.SelectedModelPath); selected != "" {
		return selected, "selected_model_path_override"
	}
	if status.Contracts != nil {
		if resolved := normalizeModelRecommendationPath(repoRoot, status.Contracts.ResolvedModelPath); resolved != "" {
			if isSupportedModelPath(resolved) {
				return resolved, "resolved_model_path"
			}
		}
	}

	supported := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Supported || !candidate.InsideRepo {
			continue
		}
		supported = append(supported, candidate.Path)
	}
	supported = uniqueSortedStrings(supported)
	switch len(supported) {
	case 0:
		return "", "no_ready_supported_model_artifact"
	case 1:
		return supported[0], "single_supported_candidate"
	default:
		return supported[0], "ambiguous_supported_candidates_lexical_fallback"
	}
}

func orderedCandidatePaths(candidates []modelRecommendationCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}
	return uniqueSortedStrings(paths)
}

func ensureValuePresent(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	values = append(values, value)
	return uniqueSortedStrings(values)
}

func evaluateModelRecommendationCandidate(repoRoot, raw string, sourceWeight int) modelRecommendationCandidate {
	path := normalizeModelRecommendationPath(repoRoot, raw)
	if path == "" {
		return modelRecommendationCandidate{}
	}

	absolute := filepath.FromSlash(path)
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(repoRoot, absolute)
	}
	absolute = filepath.Clean(absolute)

	return modelRecommendationCandidate{
		Path:         path,
		Supported:    isSupportedModelPath(path),
		InsideRepo:   isPathWithinRepo(repoRoot, absolute),
		SourceWeight: sourceWeight,
	}
}

func normalizeModelRecommendationPath(repoRoot, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	path := filepath.FromSlash(trimmed)
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	path = filepath.Clean(path)

	if isPathWithinRepo(repoRoot, path) {
		rel, err := filepath.Rel(repoRoot, path)
		if err == nil {
			return filepath.ToSlash(filepath.Clean(rel))
		}
	}

	return filepath.ToSlash(path)
}

func isSupportedModelPath(path string) bool {
	_, ok := supportedModelAuthoringExtensions[strings.ToLower(filepath.Ext(strings.TrimSpace(path)))]
	return ok
}

func isPathWithinRepo(repoRoot, path string) bool {
	repo := filepath.Clean(repoRoot)
	target := filepath.Clean(path)

	rel, err := filepath.Rel(repo, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..") && rel != ""
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := map[string]string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := unique[key]; exists {
			continue
		}
		unique[key] = trimmed
	}
	if len(unique) == 0 {
		return nil
	}
	result := make([]string, 0, len(unique))
	for _, value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i])
		right := strings.ToLower(result[j])
		if left != right {
			return left < right
		}
		return result[i] < result[j]
	})
	return result
}

func truncateCandidatePaths(values []string, preserve string, limit int) []string {
	unique := uniqueSortedStrings(values)
	if limit <= 0 || len(unique) <= limit {
		return unique
	}

	preserve = strings.TrimSpace(preserve)
	truncated := append([]string(nil), unique[:limit]...)
	if preserve == "" {
		return truncated
	}

	for _, value := range truncated {
		if strings.EqualFold(value, preserve) {
			return truncated
		}
	}

	if limit == 1 {
		return []string{preserve}
	}

	truncated = append(truncated[:limit-1], preserve)
	return uniqueSortedStrings(truncated)
}
