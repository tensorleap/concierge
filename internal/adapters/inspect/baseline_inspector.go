package inspect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// BaselineInspector performs deterministic Layer 1 artifact inventory checks.
type BaselineInspector struct{}

// NewBaselineInspector creates a baseline inspector adapter.
func NewBaselineInspector() *BaselineInspector {
	return &BaselineInspector{}
}

// Inspect validates baseline integration artifacts and readiness probes for the given snapshot.
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

	var contract *leapYAMLContract
	if !hasLeapYAML {
		status.Missing = append(status.Missing, "leap.yaml")
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeLeapYAMLMissing,
			"leap.yaml is required at repository root",
			core.IssueScopeLeapYAML,
		))
	} else {
		contract, err = inspectLeapYAMLContract(repoRoot, leapYAMLPath, &status)
		if err != nil {
			return core.IntegrationStatus{}, err
		}
		if err := inspectIntegrationContracts(repoRoot, contract, &status); err != nil {
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

	if err := inspectModelContract(repoRoot, contract, snapshot.SelectedModelPath, &status); err != nil {
		return core.IntegrationStatus{}, err
	}
	inspectInputEncoderContract(repoRoot, &status)
	inspectGTEncoderContract(repoRoot, &status)
	inspectRuntimeContract(snapshot, &status)
	inspectLeapCLIContract(snapshot, &status)

	return status, nil
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
