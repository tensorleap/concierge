package inspect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectModelContract(repoRoot string, contract *leapYAMLContract, status *core.IntegrationStatus) error {
	if contract == nil {
		return nil
	}

	modelPath := strings.TrimSpace(contract.ModelPath)
	if modelPath == "" {
		modelPath = strings.TrimSpace(contract.Model)
	}
	if modelPath == "" {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(modelPath))
	if ext != ".onnx" && ext != ".h5" {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeModelFormatUnsupported,
			Message:  fmt.Sprintf("model path %q has unsupported format; expected .onnx or .h5", modelPath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
	}

	modelAbsPath := modelPath
	if !filepath.IsAbs(modelAbsPath) {
		modelAbsPath = filepath.Join(repoRoot, filepath.FromSlash(modelPath))
	}
	modelAbsPath = filepath.Clean(modelAbsPath)

	if !isPathWithinRepo(repoRoot, modelAbsPath) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeModelFileMissing,
			Message:  fmt.Sprintf("model path %q points outside repository", modelPath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
		return nil
	}

	if info, err := os.Stat(modelAbsPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.Issues = append(status.Issues, core.Issue{
				Code:     core.IssueCodeModelFileMissing,
				Message:  fmt.Sprintf("model file %q was not found", modelPath),
				Severity: core.SeverityError,
				Scope:    core.IssueScopeModel,
			})
			return nil
		}
		return core.WrapError(core.KindUnknown, "inspect.baseline.model_stat", err)
	} else if info.IsDir() {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeModelFileMissing,
			Message:  fmt.Sprintf("model path %q must reference a file", modelPath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
		return nil
	}

	modelRelativePath, err := filepath.Rel(repoRoot, modelAbsPath)
	if err != nil {
		return core.WrapError(core.KindUnknown, "inspect.baseline.model_rel", err)
	}
	modelRelativePath = filepath.ToSlash(filepath.Clean(modelRelativePath))

	if len(contract.Include) > 0 && !matchesAnyPattern(modelRelativePath, contract.Include) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeModelPathNotIncluded,
			Message:  fmt.Sprintf("model file %q is not covered by leap.yaml include patterns", modelRelativePath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
	}
	if matchesAnyPattern(modelRelativePath, contract.Exclude) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeModelPathNotIncluded,
			Message:  fmt.Sprintf("model file %q is excluded by leap.yaml patterns", modelRelativePath),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeModel,
		})
	}

	return nil
}
