package inspect

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const maxModelAcquisitionLeadHints = 8

var (
	modelArtifactURLPattern         = regexp.MustCompile(`https?://[^\s"'<>]+?\.(?:onnx|h5|pt|pth)\b`)
	supportedArtifactMentionPattern = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9._-]*\.(?:onnx|h5)\b`)
)

type scoredModelAcquisitionLead struct {
	value string
	score int
}

type modelAcquisitionLeadCollector struct {
	byValue map[string]scoredModelAcquisitionLead
}

func discoverModelAcquisitionLeads(repoRoot string) ([]string, error) {
	collector := modelAcquisitionLeadCollector{
		byValue: make(map[string]scoredModelAcquisitionLead),
	}

	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipModelAcquisitionLeadDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isPotentialModelAcquisitionLeadFile(entry.Name()) {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." {
			return nil
		}

		if isHighValueModelAcquisitionLeadPath(rel) {
			collector.add(rel, 90)
		}

		info, err := entry.Info()
		if err == nil && info.Size() > 2*1024*1024 {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		text := string(raw)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		lowerText := strings.ToLower(text)

		if containsModelAcquisitionKeywords(lowerText) {
			collector.add(rel, 70)
		}

		for _, url := range uniqueSortedStrings(modelArtifactURLPattern.FindAllString(text, -1)) {
			collector.add(rel+" -> "+strings.TrimSpace(url), 100)
		}

		if fileSuggestsPublicExampleModel(rel, lowerText) {
			matches := uniqueSortedStrings(supportedArtifactMentionPattern.FindAllString(text, -1))
			for _, match := range matches {
				collector.add(rel+" -> "+strings.TrimSpace(match), 80)
			}
		}

		return nil
	})
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.model_acquisition.leads", err)
	}

	return collector.list(maxModelAcquisitionLeadHints), nil
}

func (c *modelAcquisitionLeadCollector) add(value string, score int) {
	if c == nil {
		return
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	key := strings.ToLower(trimmed)
	existing, ok := c.byValue[key]
	if ok && existing.score >= score {
		return
	}
	c.byValue[key] = scoredModelAcquisitionLead{
		value: trimmed,
		score: score,
	}
}

func (c *modelAcquisitionLeadCollector) list(limit int) []string {
	if c == nil || len(c.byValue) == 0 {
		return nil
	}
	values := make([]scoredModelAcquisitionLead, 0, len(c.byValue))
	for _, lead := range c.byValue {
		values = append(values, lead)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].score != values[j].score {
			return values[i].score > values[j].score
		}
		return strings.ToLower(values[i].value) < strings.ToLower(values[j].value)
	})
	if limit > 0 && len(values) > limit {
		values = values[:limit]
	}
	result := make([]string, 0, len(values))
	for _, lead := range values {
		result = append(result, lead.value)
	}
	return result
}

func shouldSkipModelAcquisitionLeadDir(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == ".concierge" {
		return true
	}
	return shouldSkipModelScanDir(trimmed)
}

func isPotentialModelAcquisitionLeadFile(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "dockerfile") {
		return true
	}
	switch filepath.Ext(lower) {
	case ".py", ".sh", ".md", ".txt", ".rst", ".yaml", ".yml", ".toml", ".ipynb":
		return true
	default:
		return false
	}
}

func isHighValueModelAcquisitionLeadPath(rel string) bool {
	lower := strings.ToLower(strings.TrimSpace(rel))
	switch lower {
	case "project_config.yaml", "project_config.yml":
		return true
	}
	return strings.Contains(lower, "export") ||
		strings.Contains(lower, "download_weights") ||
		strings.Contains(lower, "materialize") ||
		strings.Contains(lower, "/dockerfile")
}

func containsModelAcquisitionKeywords(lowerText string) bool {
	return strings.Contains(lowerText, "format=\"onnx\"") ||
		strings.Contains(lowerText, "format='onnx'") ||
		strings.Contains(lowerText, "mode=export") ||
		strings.Contains(lowerText, "export_onnx") ||
		strings.Contains(lowerText, "attempt_download_asset") ||
		(strings.Contains(lowerText, ".onnx") && strings.Contains(lowerText, "download")) ||
		(strings.Contains(lowerText, ".pt") && strings.Contains(lowerText, "download"))
}

func fileSuggestsPublicExampleModel(rel, lowerText string) bool {
	lowerRel := strings.ToLower(strings.TrimSpace(rel))
	return strings.Contains(lowerRel, "tutorial") ||
		strings.Contains(lowerRel, "example") ||
		strings.Contains(lowerRel, "readme") ||
		strings.Contains(lowerRel, "dockerfile") ||
		strings.Contains(lowerText, "public model") ||
		strings.Contains(lowerText, "example usage")
}
