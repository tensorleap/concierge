package inspect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	validateadapter "github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/core"
)

// BaselineInspector performs deterministic Layer 1 artifact inventory checks.
type BaselineInspector struct {
	integrationTestAnalyzer integrationTestAnalyzer
}

type integrationTestAnalyzer interface {
	Analyze(ctx context.Context, snapshot core.WorkspaceSnapshot) (validateadapter.IntegrationTestASTResult, error)
}

// NewBaselineInspector creates a baseline inspector adapter.
func NewBaselineInspector() *BaselineInspector {
	return &BaselineInspector{
		integrationTestAnalyzer: validateadapter.NewIntegrationTestASTAnalyzer(),
	}
}

// Inspect validates baseline integration artifacts and readiness probes for the given snapshot.
func (i *BaselineInspector) Inspect(ctx context.Context, snapshot core.WorkspaceSnapshot) (core.IntegrationStatus, error) {
	_ = ctx

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.IntegrationStatus{}, core.NewError(core.KindUnknown, "inspect.baseline.root", "snapshot repository root is empty")
	}

	status := core.IntegrationStatus{}

	canonicalEntryPath := filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile)
	hasCanonicalEntry, err := fileExists(canonicalEntryPath)
	if err != nil {
		return core.IntegrationStatus{}, core.WrapError(core.KindUnknown, "inspect.baseline.integration_entry_exists", err)
	}
	if !hasCanonicalEntry {
		status.Missing = append(status.Missing, core.CanonicalIntegrationEntryFile)
		status.Issues = append(status.Issues, requiredArtifactIssue(
			core.IssueCodeIntegrationScriptMissing,
			fmt.Sprintf("%s is required at repository root", core.CanonicalIntegrationEntryFile),
			core.IssueScopeIntegrationScript,
		))
	}

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
		inspectCanonicalIntegrationLayout(contract, &status)
		inspectRequiredUploadBoundary(repoRoot, contract, &status)
		if err := inspectIntegrationContracts(repoRoot, contract, &status); err != nil {
			return core.IntegrationStatus{}, err
		}
	}
	if err := i.inspectIntegrationTestWiring(ctx, snapshot, &status); err != nil {
		return core.IntegrationStatus{}, err
	}

	if err := inspectModelAcquisition(snapshot, repoRoot, contract, snapshot.SelectedModelPath, &status); err != nil {
		return core.IntegrationStatus{}, err
	}
	if err := inspectModelContract(repoRoot, contract, snapshot.SelectedModelPath, &status); err != nil {
		return core.IntegrationStatus{}, err
	}
	inspectModelUploadBoundary(snapshot, contract, &status)
	if status.Contracts == nil && !hasLeapYAML {
		status.Contracts = &core.IntegrationContracts{}
	}
	if err := inspectInputGTDiscovery(ctx, snapshot, &status); err != nil {
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

func inspectCanonicalIntegrationLayout(contract *leapYAMLContract, status *core.IntegrationStatus) {
	if contract == nil || status == nil {
		return
	}

	entryFile := filepath.ToSlash(filepath.Clean(strings.TrimSpace(contract.EntryFile)))
	entryFile = strings.TrimPrefix(entryFile, "./")
	if entryFile == "" || entryFile == core.CanonicalIntegrationEntryFile {
		return
	}

	status.Issues = append(status.Issues, core.Issue{
		Code:     core.IssueCodeIntegrationScriptNonCanonical,
		Message:  fmt.Sprintf("Concierge only supports %q as leap.yaml entryFile; found %q", core.CanonicalIntegrationEntryFile, entryFile),
		Severity: core.SeverityError,
		Scope:    core.IssueScopeIntegrationScript,
		Location: &core.IssueLocation{
			Path:   "leap.yaml",
			Symbol: "entryFile",
		},
	})
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

func (i *BaselineInspector) inspectIntegrationTestWiring(
	ctx context.Context,
	snapshot core.WorkspaceSnapshot,
	status *core.IntegrationStatus,
) error {
	if status == nil || status.Contracts == nil || len(status.Contracts.IntegrationTestFunctions) == 0 {
		return nil
	}
	if snapshot.RuntimeProfile == nil || strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath) == "" {
		return nil
	}
	if i == nil || i.integrationTestAnalyzer == nil {
		return nil
	}
	if _, err := os.Stat(strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return core.WrapError(core.KindUnknown, "inspect.baseline.integration_test_ast.interpreter", err)
	}

	result, err := i.integrationTestAnalyzer.Analyze(ctx, snapshot)
	if err != nil {
		return core.WrapError(core.KindUnknown, "inspect.baseline.integration_test_ast", err)
	}
	status.Issues = append(status.Issues, result.Issues...)
	return nil
}
