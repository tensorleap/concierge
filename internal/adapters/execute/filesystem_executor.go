package execute

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"gopkg.in/yaml.v3"
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
		return ensureLeapYAML(repoRoot, canonicalStep)
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

func ensureLeapYAML(repoRoot string, step core.EnsureStep) (core.ExecutionResult, error) {
	targetPath := filepath.Join(repoRoot, "leap.yaml")
	beforeChecksum, beforeState, err := checksumForPath(targetPath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.before_checksum", err)
	}

	if beforeState == "missing" {
		return applyTemplate(repoRoot, step, "leap.yaml", "templates/leap_yaml.tmpl")
	}

	raw, err := os.ReadFile(targetPath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.leap_yaml_read", err)
	}

	reconciled, changed, reason, err := reconcileLeapYAML(raw, repoRoot)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.leap_yaml_reconcile", err)
	}

	if changed {
		if err := os.WriteFile(targetPath, reconciled, 0o644); err != nil {
			return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.leap_yaml_write", err)
		}
	}

	afterChecksum, _, err := checksumForPath(targetPath)
	if err != nil {
		return core.ExecutionResult{}, core.WrapError(core.KindUnknown, "execute.filesystem.after_checksum", err)
	}

	summary := "leap.yaml already satisfies required upload rules"
	if changed {
		summary = reason
	}

	result := core.ExecutionResult{
		Step:    step,
		Applied: changed,
		Summary: summary,
		Evidence: []core.EvidenceItem{
			{Name: "executor.mode", Value: "filesystem"},
			{Name: "executor.target_path", Value: "leap.yaml"},
			{Name: "executor.before_checksum", Value: beforeChecksum},
			{Name: "executor.after_checksum", Value: afterChecksum},
		},
	}

	return result, nil
}

func reconcileLeapYAML(contents []byte, repoRoot string) ([]byte, bool, string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(contents, &document); err != nil {
		templateContents, readErr := templateFS.ReadFile("templates/leap_yaml.tmpl")
		if readErr != nil {
			return nil, false, "", readErr
		}
		return templateContents, true, "replaced invalid leap.yaml with a baseline template", nil
	}
	if len(document.Content) == 0 {
		templateContents, readErr := templateFS.ReadFile("templates/leap_yaml.tmpl")
		if readErr != nil {
			return nil, false, "", readErr
		}
		return templateContents, true, "replaced empty leap.yaml with a baseline template", nil
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		templateContents, readErr := templateFS.ReadFile("templates/leap_yaml.tmpl")
		if readErr != nil {
			return nil, false, "", readErr
		}
		return templateContents, true, "replaced invalid leap.yaml with a baseline template", nil
	}

	changed := false
	entryAdjusted := false
	includeAdjusted := false
	excludeAdjusted := false

	entryNode := getOrCreateMappingValue(root, "entryFile", &changed)
	entryFile := normalizeUploadPath(entryNode.Value)
	if entryFile == "" {
		entryFile = "leap_binder.py"
		entryNode.Value = entryFile
		changed = true
		entryAdjusted = true
	}

	if filepath.IsAbs(entryFile) {
		entryFile = "leap_binder.py"
		entryNode.Value = entryFile
		changed = true
		entryAdjusted = true
	}

	entryAbs := filepath.Join(repoRoot, filepath.FromSlash(entryFile))
	entryInfo, statErr := os.Stat(entryAbs)
	if statErr != nil || entryInfo.IsDir() {
		if !errors.Is(statErr, os.ErrNotExist) || entryFile != "leap_binder.py" || entryInfo != nil {
			entryFile = "leap_binder.py"
			entryNode.Value = entryFile
			changed = true
			entryAdjusted = true
		}
	}

	required := requiredLeapYAMLPaths(repoRoot, entryFile)

	includeNode, includeExists := findMappingValue(root, "include")
	if includeExists {
		if includeNode.Kind != yaml.SequenceNode {
			includeNode.Kind = yaml.SequenceNode
			includeNode.Tag = "!!seq"
			includeNode.Content = nil
			changed = true
			includeAdjusted = true
		}

		includePatterns := sequenceValues(includeNode)
		for _, req := range required {
			if matchesAnyPattern(req, includePatterns) {
				continue
			}
			includeNode.Content = append(includeNode.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: req,
			})
			includePatterns = append(includePatterns, req)
			changed = true
			includeAdjusted = true
		}
	}

	excludeNode, excludeExists := findMappingValue(root, "exclude")
	if excludeExists {
		if excludeNode.Kind != yaml.SequenceNode {
			excludeNode.Kind = yaml.SequenceNode
			excludeNode.Tag = "!!seq"
			excludeNode.Content = nil
			changed = true
			excludeAdjusted = true
		}

		filtered := make([]*yaml.Node, 0, len(excludeNode.Content))
		for _, item := range excludeNode.Content {
			if item == nil {
				continue
			}
			pattern := strings.TrimSpace(item.Value)
			if pattern == "" {
				filtered = append(filtered, item)
				continue
			}

			blocksRequired := false
			for _, req := range required {
				if matchesPattern(req, pattern) {
					blocksRequired = true
					break
				}
			}
			if blocksRequired {
				changed = true
				excludeAdjusted = true
				continue
			}
			filtered = append(filtered, item)
		}
		excludeNode.Content = filtered
	}

	if !changed {
		return contents, false, "", nil
	}

	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return nil, false, "", err
	}

	switch {
	case entryAdjusted && (includeAdjusted || excludeAdjusted):
		return encoded, true, "updated leap.yaml entryFile and upload rules", nil
	case entryAdjusted:
		return encoded, true, "updated leap.yaml entryFile", nil
	case includeAdjusted || excludeAdjusted:
		return encoded, true, "updated leap.yaml upload rules", nil
	default:
		return encoded, true, "updated leap.yaml", nil
	}
}

func requiredLeapYAMLPaths(repoRoot string, entryFile string) []string {
	required := []string{"leap.yaml", normalizeUploadPath(entryFile)}
	appendIfExists := func(name string) {
		info, err := os.Stat(filepath.Join(repoRoot, name))
		if err != nil || info.IsDir() {
			return
		}
		required = append(required, normalizeUploadPath(name))
	}
	appendIfExists("leap_binder.py")
	appendIfExists("leap_custom_test.py")
	appendIfExists("integration_test.py")
	return dedupeStrings(required)
}

func findMappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		v := mapping.Content[i+1]
		if strings.TrimSpace(k.Value) == key {
			return v, true
		}
	}
	return nil, false
}

func getOrCreateMappingValue(mapping *yaml.Node, key string, changed *bool) *yaml.Node {
	if existing, ok := findMappingValue(mapping, key); ok {
		return existing
	}

	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}
	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: "",
	}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	if changed != nil {
		*changed = true
	}
	return valueNode
}

func sequenceValues(sequence *yaml.Node) []string {
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return nil
	}
	values := make([]string, 0, len(sequence.Content))
	for _, item := range sequence.Content {
		if item == nil {
			continue
		}
		trimmed := normalizeUploadPath(item.Value)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func matchesAnyPattern(path string, patterns []string) bool {
	normalizedPath := normalizeUploadPath(path)
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

func normalizeUploadPath(path string) string {
	normalized := strings.TrimSpace(path)
	normalized = filepath.ToSlash(filepath.Clean(normalized))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "." {
		return ""
	}
	return normalized
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := normalizeUploadPath(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
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
