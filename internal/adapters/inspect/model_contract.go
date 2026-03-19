package inspect

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelContract(repoRoot string, contract *leapYAMLContract, selectedModelPath string, status *core.IntegrationStatus) error {
	_ = selectedModelPath
	if contract == nil || status == nil || status.Contracts == nil {
		return nil
	}
	if strings.TrimSpace(status.Contracts.EntryFile) == "" {
		return nil
	}

	loadModelFunctions := uniqueSymbols(status.Contracts.LoadModelFunctions)
	if len(loadModelFunctions) == 0 {
		appendModelIssue(
			status,
			core.IssueCodeLoadModelDecoratorMissing,
			fmt.Sprintf("no @tensorleap_load_model function found in %s", strings.TrimSpace(status.Contracts.EntryFile)),
			core.SeverityError,
		)
		return nil
	}

	resolved := strings.TrimSpace(status.Contracts.ResolvedModelPath)
	if resolved == "" {
		return nil
	}

	evaluation, err := evaluateModelCandidate(repoRoot, core.ModelCandidate{
		Path:   resolved,
		Source: "model_contract.resolved",
	})
	if err != nil {
		return err
	}
	switch {
	case !evaluation.SupportedFormat:
		appendModelIssue(
			status,
			core.IssueCodeModelFormatUnsupported,
			fmt.Sprintf("resolved model artifact %q is not a supported .onnx or .h5 file", strings.TrimSpace(evaluation.DisplayPath)),
			core.SeverityError,
		)
	case !evaluation.InsideRepo:
		appendModelIssue(
			status,
			core.IssueCodeModelFileMissing,
			fmt.Sprintf("resolved model artifact %q points outside the repository", strings.TrimSpace(evaluation.Candidate.Path)),
			core.SeverityError,
		)
	case !evaluation.Exists:
		appendModelIssue(
			status,
			core.IssueCodeModelFileMissing,
			fmt.Sprintf("resolved model artifact %q was not found", strings.TrimSpace(evaluation.DisplayPath)),
			core.SeverityError,
		)
	default:
		status.Contracts.ResolvedModelPath = filepath.ToSlash(filepath.Clean(evaluation.DisplayPath))
	}

	return nil
}

func appendModelIssue(status *core.IntegrationStatus, code core.IssueCode, message string, severity core.Severity) {
	if status == nil {
		return
	}
	status.Issues = append(status.Issues, core.Issue{
		Code:     code,
		Message:  message,
		Severity: severity,
		Scope:    core.IssueScopeModel,
	})
}
