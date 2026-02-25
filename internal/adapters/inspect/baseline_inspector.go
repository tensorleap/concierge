package inspect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tensorleap/concierge/internal/core"
)

// BaselineInspector performs deterministic Layer 1 artifact inventory checks.
type BaselineInspector struct{}

// NewBaselineInspector creates a baseline inspector adapter.
func NewBaselineInspector() *BaselineInspector {
	return &BaselineInspector{}
}

type leapYAMLDocument struct {
	EntryFile string `yaml:"entryFile"`
}

// Inspect validates the baseline integration artifacts for the given snapshot.
func (i *BaselineInspector) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.IntegrationStatus{}, core.NewError(core.KindUnknown, "inspect.baseline.root", "snapshot repository root is empty")
	}

	status := core.IntegrationStatus{}

	leapYAMLPath := filepath.Join(repoRoot, "leap.yaml")
	hasLeapYAML, err := fileExists(leapYAMLPath)
	if err != nil {
		return core.IntegrationStatus{}, core.WrapError(core.KindUnknown, "inspect.baseline.leap_yaml_exists", err)
	}
	if !hasLeapYAML {
		status.Missing = append(status.Missing, "leap.yaml")
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLMissing,
			"leap.yaml is required at repository root",
			core.IssueScopeLeapYAML,
		))
	} else {
		if err := inspectLeapYAML(repoRoot, leapYAMLPath, &status); err != nil {
			return core.IntegrationStatus{}, err
		}
	}

	binderPath := filepath.Join(repoRoot, "leap_binder.py")
	hasBinder, err := fileExists(binderPath)
	if err != nil {
		return core.IntegrationStatus{}, core.WrapError(core.KindUnknown, "inspect.baseline.binder_exists", err)
	}
	if !hasBinder {
		status.Missing = append(status.Missing, "leap_binder.py")
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeIntegrationScriptMissing,
			"leap_binder.py is required at repository root",
			core.IssueScopeIntegrationScript,
		))
	}

	hasCustomTest, err := fileExists(filepath.Join(repoRoot, "leap_custom_test.py"))
	if err != nil {
		return core.IntegrationStatus{}, core.WrapError(core.KindUnknown, "inspect.baseline.custom_test_exists", err)
	}
	hasIntegrationTest, err := fileExists(filepath.Join(repoRoot, "integration_test.py"))
	if err != nil {
		return core.IntegrationStatus{}, core.WrapError(core.KindUnknown, "inspect.baseline.integration_test_exists", err)
	}
	if !hasCustomTest && !hasIntegrationTest {
		status.Missing = append(status.Missing, "integration_test")
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeIntegrationTestMissing,
			"either leap_custom_test.py or integration_test.py is required at repository root",
			core.IssueScopeIntegrationTest,
		))
	}

	return status, nil
}

func inspectLeapYAML(repoRoot string, leapYAMLPath string, status *core.IntegrationStatus) error {
	contents, err := os.ReadFile(leapYAMLPath)
	if err != nil {
		return core.WrapError(core.KindUnknown, "inspect.baseline.leap_yaml_read", err)
	}

	var document leapYAMLDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLUnparseable,
			fmt.Sprintf("leap.yaml is not parseable: %v", err),
			core.IssueScopeLeapYAML,
		))
		return nil
	}

	entryFile := strings.TrimSpace(document.EntryFile)
	if entryFile == "" {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileMissing,
			"leap.yaml must define a non-empty entryFile",
			core.IssueScopeLeapYAML,
		))
		return nil
	}

	entryPath := filepath.Join(repoRoot, filepath.FromSlash(entryFile))
	entryInfo, err := os.Stat(entryPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.Issues = append(status.Issues, requiredArtifactIssue(
				core.IssueCodeLeapYAMLEntryFileNotFound,
				fmt.Sprintf("leap.yaml entryFile %q was not found", entryFile),
				core.IssueScopeLeapYAML,
			))
			return nil
		}
		return core.WrapError(core.KindUnknown, "inspect.baseline.entry_file_stat", err)
	}
	if entryInfo.IsDir() {
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLEntryFileNotFound,
			fmt.Sprintf("leap.yaml entryFile %q must reference a file", entryFile),
			core.IssueScopeLeapYAML,
		))
	}

	return nil
}

func requiredArtifactIssue(code core.IssueCode, message string, scope core.IssueScope) core.Issue {
	return core.Issue{
		Code:     code,
		Message:  message,
		Severity: core.SeverityError,
		Scope:    scope,
	}
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
