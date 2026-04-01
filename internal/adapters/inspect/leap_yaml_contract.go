package inspect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tensorleap/concierge/internal/core"
)

type leapYAMLContract struct {
	EntryFile     string   `yaml:"entryFile"`
	Include       []string `yaml:"include"`
	Exclude       []string `yaml:"exclude"`
	ProjectID     string   `yaml:"projectId"`
	SecretID      string   `yaml:"secretId"`
	PythonVersion string   `yaml:"pythonVersion"`
}

func inspectLeapYAMLContract(repoRoot string, leapYAMLPath string, status *core.IntegrationStatus) (*leapYAMLContract, error) {
	contents, err := os.ReadFile(leapYAMLPath)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.leap_yaml_read", err)
	}

	var contract leapYAMLContract
	if err := yaml.Unmarshal(contents, &contract); err != nil {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLUnparseable,
			fmt.Sprintf("leap.yaml is not parseable: %v", err),
			core.IssueScopeLeapYAML,
		))
		return nil, nil
	}

	entryFile := strings.TrimSpace(contract.EntryFile)
	if entryFile == "" {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileMissing,
			"leap.yaml must define a non-empty entryFile",
			core.IssueScopeLeapYAML,
		))
		return &contract, nil
	}

	entryAbsPath := entryFile
	if !filepath.IsAbs(entryAbsPath) {
		entryAbsPath = filepath.Join(repoRoot, filepath.FromSlash(entryFile))
	}
	entryAbsPath = filepath.Clean(entryAbsPath)

	if !isPathWithinRepo(repoRoot, entryAbsPath) {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileOutsideRepo,
			fmt.Sprintf("leap.yaml entryFile %q points outside repository", entryFile),
			core.IssueScopeLeapYAML,
		))
		return &contract, nil
	}

	entryInfo, err := os.Stat(entryAbsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.Issues = append(status.Issues, requiredArtifactIssue(
				core.IssueCodeLeapYAMLEntryFileNotFound,
				fmt.Sprintf("leap.yaml entryFile %q was not found", entryFile),
				core.IssueScopeLeapYAML,
			))
			return &contract, nil
		}
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.entry_file_stat", err)
	}
	if entryInfo.IsDir() {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileInvalid,
			fmt.Sprintf("leap.yaml entryFile %q must reference a file", entryFile),
			core.IssueScopeLeapYAML,
		))
		return &contract, nil
	}

	return &contract, nil
}

func isPathWithinRepo(repoRoot string, path string) bool {
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

func inspectModelUploadBoundary(snapshot core.WorkspaceSnapshot, contract *leapYAMLContract, status *core.IntegrationStatus) {
	if contract == nil || status == nil {
		return
	}

	requiredModelPath := strings.TrimSpace(snapshot.SelectedModelPath)
	if requiredModelPath == "" && status.Contracts != nil {
		requiredModelPath = strings.TrimSpace(status.Contracts.ResolvedModelPath)
	}
	requiredModelPath = normalizeUploadBoundaryPath(requiredModelPath)
	if requiredModelPath == "" || !strings.HasPrefix(requiredModelPath, ".concierge/materialized_models/") {
		return
	}

	includePatterns := normalizeUploadBoundaryPatterns(contract.Include)
	if !uploadBoundaryMatchesAnyPattern(requiredModelPath, includePatterns) {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLIncludeMissingRequiredFiles,
			fmt.Sprintf("leap.yaml include list does not include required model artifact %q", requiredModelPath),
			core.IssueScopeLeapYAML,
		))
	}

	excludePatterns := normalizeUploadBoundaryPatterns(contract.Exclude)
	if uploadBoundaryExplicitlyBlocksRequiredPath(requiredModelPath, excludePatterns) {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLExcludeBlocksRequiredFiles,
			fmt.Sprintf("leap.yaml exclude list blocks required model artifact %q", requiredModelPath),
			core.IssueScopeLeapYAML,
		))
	}
}

func normalizeUploadBoundaryPatterns(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeUploadBoundaryPath(value)
		if normalized == "" {
			continue
		}
		result = append(result, normalized)
	}
	return result
}

func uploadBoundaryMatchesAnyPattern(path string, patterns []string) bool {
	normalizedPath := normalizeUploadBoundaryPath(path)
	for _, pattern := range patterns {
		if uploadBoundaryMatchesPattern(normalizedPath, pattern) {
			return true
		}
	}
	return false
}

func uploadBoundaryMatchesPattern(path string, pattern string) bool {
	normalizedPattern := normalizeUploadBoundaryPath(pattern)
	if normalizedPattern == "" {
		return false
	}

	if strings.HasSuffix(normalizedPattern, "/**") {
		prefix := strings.TrimSuffix(normalizedPattern, "/**")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	if strings.HasSuffix(normalizedPattern, "/*") {
		prefix := strings.TrimSuffix(normalizedPattern, "/*")
		if !strings.HasPrefix(path, prefix+"/") {
			return false
		}
		remaining := strings.TrimPrefix(path, prefix+"/")
		return !strings.Contains(remaining, "/")
	}

	matched, err := filepath.Match(normalizedPattern, path)
	if err == nil && matched {
		return true
	}

	if !strings.ContainsAny(normalizedPattern, "*?[") {
		return path == normalizedPattern
	}
	return false
}

func uploadBoundaryExplicitlyBlocksRequiredPath(path string, patterns []string) bool {
	normalizedPath := normalizeUploadBoundaryPath(path)
	for _, pattern := range patterns {
		if pattern == normalizedPath {
			return true
		}
		if pattern == ".concierge/**" && strings.HasPrefix(normalizedPath, ".concierge/") {
			return true
		}
	}
	return false
}

func normalizeUploadBoundaryPath(path string) string {
	normalized := strings.TrimSpace(path)
	normalized = filepath.ToSlash(filepath.Clean(normalized))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "." {
		return ""
	}
	return normalized
}
