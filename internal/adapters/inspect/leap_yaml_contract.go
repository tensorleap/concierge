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
