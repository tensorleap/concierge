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
	ModelPath     string   `yaml:"modelPath"`
	Model         string   `yaml:"model"`
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

	entryRelativePath, err := filepath.Rel(repoRoot, entryAbsPath)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.entry_file_rel", err)
	}
	entryRelativePath = filepath.ToSlash(filepath.Clean(entryRelativePath))

	requiredPaths := requiredUploadPaths(repoRoot, entryRelativePath)
	missingIncludes := make([]string, 0)
	excludedRequired := make([]string, 0)
	entryExcluded := false
	for _, required := range requiredPaths {
		if len(contract.Include) > 0 && !matchesAnyPattern(required, contract.Include) {
			missingIncludes = append(missingIncludes, required)
		}
		if matchesAnyPattern(required, contract.Exclude) {
			excludedRequired = append(excludedRequired, required)
			if required == entryRelativePath {
				entryExcluded = true
			}
		}
	}

	if len(missingIncludes) > 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLIncludeMissingRequiredFiles,
			Message:  fmt.Sprintf("leap.yaml include patterns miss required files: %s", strings.Join(missingIncludes, ", ")),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeLeapYAML,
		})
	}

	if len(excludedRequired) > 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLExcludeBlocksRequiredFiles,
			Message:  fmt.Sprintf("leap.yaml exclude patterns block required files: %s", strings.Join(excludedRequired, ", ")),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeLeapYAML,
		})
	}

	if entryExcluded {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileExcluded,
			fmt.Sprintf("leap.yaml entryFile %q is excluded by upload patterns", entryRelativePath),
			core.IssueScopeLeapYAML,
		))
	}

	if strings.TrimSpace(contract.ProjectID) != "" {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLProjectIDSetInitialSetup,
			Message:  "leap.yaml sets projectId; initial setup typically leaves project identifiers empty",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeLeapYAML,
		})
	}
	if strings.TrimSpace(contract.SecretID) != "" {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLSecretIDSetInitialSetup,
			Message:  "leap.yaml sets secretId; initial setup typically leaves project identifiers empty",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeLeapYAML,
		})
	}

	return &contract, nil
}

func requiredUploadPaths(repoRoot string, entryRelativePath string) []string {
	required := []string{"leap.yaml", entryRelativePath}
	appendIfExists := func(name string) {
		if info, err := os.Stat(filepath.Join(repoRoot, name)); err == nil && !info.IsDir() {
			required = append(required, filepath.ToSlash(name))
		}
	}
	appendIfExists("leap_binder.py")
	appendIfExists("leap_custom_test.py")
	appendIfExists("integration_test.py")
	return dedupeStrings(required)
}

func matchesAnyPattern(path string, patterns []string) bool {
	normalizedPath := filepath.ToSlash(filepath.Clean(path))
	for _, pattern := range patterns {
		if matchesPattern(normalizedPath, pattern) {
			return true
		}
	}
	return false
}

func matchesPattern(path string, pattern string) bool {
	normalizedPattern := filepath.ToSlash(strings.TrimSpace(pattern))
	normalizedPattern = strings.TrimPrefix(normalizedPattern, "./")
	normalizedPattern = strings.TrimPrefix(normalizedPattern, "/")
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

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}
