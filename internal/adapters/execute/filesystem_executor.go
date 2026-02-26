package execute

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// FilesystemExecutor applies deterministic scaffold mutations for known ensure-steps.
type FilesystemExecutor struct{}

// NewFilesystemExecutor creates a deterministic filesystem-backed executor.
func NewFilesystemExecutor() *FilesystemExecutor {
	return &FilesystemExecutor{}
}

// Execute applies supported ensure-steps and emits before/after checksum evidence.
func (e *FilesystemExecutor) Execute(ctx context.Context, snapshot core.WorkspaceSnapshot, step core.EnsureStep) (core.ExecutionResult, error) {
	_ = ctx

	repoRoot := strings.TrimSpace(snapshot.Repository.Root)
	if repoRoot == "" {
		return core.ExecutionResult{}, core.NewError(core.KindUnknown, "execute.filesystem.repo_root", "snapshot repository root is empty")
	}

	canonicalStep, ok := core.EnsureStepByID(step.ID)
	if !ok {
		return core.ExecutionResult{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.filesystem.step",
			fmt.Errorf("unknown ensure-step ID %q", step.ID),
		)
	}

	switch canonicalStep.ID {
	case core.EnsureStepLeapYAML:
		return applyTemplate(repoRoot, canonicalStep, "leap.yaml", "templates/leap_yaml.tmpl")
	case core.EnsureStepIntegrationScript:
		return applyTemplate(repoRoot, canonicalStep, "leap_binder.py", "templates/leap_binder.py.tmpl")
	case core.EnsureStepIntegrationTestContract:
		return applyTemplate(repoRoot, canonicalStep, "leap_custom_test.py", "templates/leap_custom_test.py.tmpl")
	default:
		return core.ExecutionResult{}, core.WrapError(
			core.KindStepNotApplicable,
			"execute.filesystem.unsupported_step",
			fmt.Errorf("ensure-step %q is not supported by filesystem executor", canonicalStep.ID),
		)
	}
}

func applyTemplate(repoRoot string, step core.EnsureStep, relativePath string, templatePath string) (core.ExecutionResult, error) {
	targetPath := filepath.Join(repoRoot, relativePath)
	beforeChecksum, beforeState, err := checksumForPath(targetPath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.before_checksum", err)
	}

	templateContents, err := templateFS.ReadFile(templatePath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.template_read", err)
	}

	applied := false
	summary := fmt.Sprintf("%s already exists; no changes applied", relativePath)
	if beforeState == "missing" {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.mkdir", err)
		}
		if err := os.WriteFile(targetPath, templateContents, 0o644); err != nil {
			return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.write", err)
		}
		applied = true
		summary = fmt.Sprintf("created %s", relativePath)
	}

	afterChecksum, _, err := checksumForPath(targetPath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.after_checksum", err)
	}

	result := core.ExecutionResult{
		Step:    step,
		Applied: applied,
		Summary: summary,
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "filesystem"},
			{Name: "executor.target_path", Value: relativePath},
			{Name: "executor.before_checksum", Value: beforeChecksum},
			{Name: "executor.after_checksum", Value: afterChecksum},
		},
	}

	return result, nil
}

func checksumForPath(path string) (string, string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", "missing", nil
		}
		return "", "", err
	}

	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), "present", nil
}
