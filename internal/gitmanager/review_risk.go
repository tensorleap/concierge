package gitmanager

import (
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func classifyReviewRisk(step core.EnsureStep, files []string, stat string) ChangeReviewRisk {
	artifactPaths := make([]string, 0, len(files))
	repoLogicCount := 0
	for _, line := range files {
		path := changedPathFromLine(line)
		if path == "" {
			continue
		}
		if isDatasetArtifactPath(path) {
			artifactPaths = append(artifactPaths, path)
			continue
		}
		if isReviewableRepoLogicPath(path) {
			repoLogicCount++
		}
	}
	if len(artifactPaths) == 0 {
		return ChangeReviewRisk{}
	}

	binaryArtifacts := artifactBinaryCount(stat)
	reasons := []string{"dataset/cache paths were added"}
	if binaryArtifacts > 0 {
		reasons = append(reasons, "binary artifact files were added")
	}

	risk := ChangeReviewRisk{
		Level:     ChangeReviewRiskHigh,
		Summary:   "This diff includes dataset/cache artifacts that should usually stay out of the repository working tree.",
		Reasons:   reasons,
		HidePatch: true,
	}
	if step.ID == core.EnsureStepPreprocessContract && repoLogicCount == 0 {
		risk.Block = true
		risk.Summary = "This preprocess remediation only vendored dataset/cache artifacts into the repository working tree, so Concierge blocked it."
	}
	return risk
}

func changedPathFromLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if tab := strings.Index(trimmed, "\t"); tab >= 0 {
		return strings.TrimSpace(trimmed[tab+1:])
	}
	if space := strings.Index(trimmed, " "); space >= 0 {
		return strings.TrimSpace(trimmed[space+1:])
	}
	return trimmed
}

func isDatasetArtifactPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, ".tensorleap_data/") ||
		strings.HasPrefix(lower, ".cache/") ||
		strings.HasPrefix(lower, ".venv/") ||
		strings.Contains(lower, "/site-packages/") {
		return true
	}
	if strings.Contains(lower, "/images/") || strings.Contains(lower, "/labels/") {
		switch filepath.Ext(lower) {
		case ".jpg", ".jpeg", ".png", ".bmp", ".gif", ".webp", ".tif", ".tiff", ".txt":
			return true
		}
	}
	base := filepath.Base(lower)
	if (base == "license" || base == "license.txt" || base == "readme.md" || base == "readme.txt") &&
		(strings.Contains(lower, "/datasets/") || strings.Contains(lower, ".tensorleap_data/")) {
		return true
	}
	return false
}

func isReviewableRepoLogicPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	if lower == "" || isDatasetArtifactPath(lower) {
		return false
	}
	switch filepath.Ext(lower) {
	case ".go", ".py", ".yaml", ".yml", ".json", ".toml", ".sh", ".md":
		return true
	default:
		return false
	}
}

func artifactBinaryCount(stat string) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(stat, "\r\n", "\n"), "\n") {
		if strings.Contains(line, "| Bin ") {
			count++
		}
	}
	return count
}
