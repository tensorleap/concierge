package core

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	pythonImportPattern              = regexp.MustCompile(`(?m)^\s*import\s+([^\n#]+)`)
	pythonFromImportPattern          = regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z_][A-Za-z0-9_\.]*)\s+import\s+([^\n#]+)`)
	repoRootAssignmentPattern        = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:Path|pathlib\.Path)\s*\(\s*__file__\s*\)[^\n#]*\bparent(?:\b|\s*\[)`)
	repoRootAliasAssignmentPattern   = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	repoRootPathReferencePattern     = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\b((?:\s*/\s*(?:"[^"\n]+"|'[^'\n]+'))+)`)
	repoRootPathReferencePartPattern = regexp.MustCompile(`(?:"([^"\n]+)"|'([^'\n]+)')`)
	supportedBoundaryModelExtensions = map[string]struct{}{".onnx": {}, ".h5": {}}
	defaultRepoRootReferenceVariable = []string{"_REPO_ROOT", "REPO_ROOT", "repo_root", "repoRoot"}
)

// RequiredUploadBoundaryPaths returns the deterministic set of repo files that
// must remain inside leap.yaml's upload boundary for the current entry file.
func RequiredUploadBoundaryPaths(repoRoot string, entryFile string) []string {
	required := []string{"leap.yaml", normalizeUploadBoundaryPath(entryFile)}
	for _, candidate := range RequirementsFileCandidates {
		if uploadBoundaryFileExists(filepath.Join(repoRoot, candidate)) {
			required = append(required, candidate)
		}
	}
	for _, pair := range RequirementsFilePairs {
		if uploadBoundaryFileExists(filepath.Join(repoRoot, pair[0])) && uploadBoundaryFileExists(filepath.Join(repoRoot, pair[1])) {
			required = append(required, pair[0], pair[1])
		}
	}

	entryPath := resolveUploadBoundaryEntryPath(repoRoot, entryFile)
	if entryPath == "" {
		return dedupeUploadBoundaryPaths(required)
	}

	source, err := os.ReadFile(entryPath)
	if err != nil {
		return dedupeUploadBoundaryPaths(required)
	}

	required = append(required, discoverDirectRepoImportDependencies(repoRoot, string(source))...)
	required = append(required, discoverDirectRepoPathDependencies(repoRoot, string(source))...)
	return dedupeUploadBoundaryPaths(required)
}

func resolveUploadBoundaryEntryPath(repoRoot string, entryFile string) string {
	normalized := normalizeUploadBoundaryPath(entryFile)
	if normalized == "" {
		normalized = CanonicalIntegrationEntryFile
	}
	entryPath := filepath.Join(repoRoot, filepath.FromSlash(normalized))
	if !uploadBoundaryFileExists(entryPath) {
		return ""
	}
	return entryPath
}

func discoverDirectRepoImportDependencies(repoRoot string, source string) []string {
	if strings.TrimSpace(source) == "" {
		return nil
	}

	dependencies := make([]string, 0, 8)

	for _, match := range pythonImportPattern.FindAllStringSubmatch(source, -1) {
		for _, module := range splitPythonImportTargets(match[1]) {
			if resolved := resolveRepoLocalPythonModule(repoRoot, module); resolved != "" {
				dependencies = append(dependencies, resolved)
			}
		}
	}

	for _, match := range pythonFromImportPattern.FindAllStringSubmatch(source, -1) {
		baseModule := strings.TrimSpace(match[1])
		if resolved := resolveRepoLocalPythonModule(repoRoot, baseModule); resolved != "" {
			dependencies = append(dependencies, resolved)
		}

		for _, imported := range splitPythonImportTargets(match[2]) {
			if imported == "*" {
				continue
			}
			submodule := strings.TrimSpace(baseModule + "." + imported)
			if resolved := resolveRepoLocalPythonModule(repoRoot, submodule); resolved != "" {
				dependencies = append(dependencies, resolved)
			}
		}
	}

	return dedupeUploadBoundaryPaths(dependencies)
}

func splitPythonImportTargets(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "(")
	trimmed = strings.TrimSuffix(trimmed, ")")
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		fields := strings.Fields(candidate)
		if len(fields) == 0 {
			continue
		}
		module := strings.TrimSpace(fields[0])
		if module == "" {
			continue
		}
		out = append(out, module)
	}
	return out
}

func resolveRepoLocalPythonModule(repoRoot string, module string) string {
	module = strings.TrimSpace(module)
	if module == "" || strings.HasPrefix(module, ".") {
		return ""
	}

	relPath := filepath.ToSlash(filepath.Join(strings.Split(module, ".")...))
	if relPath == "" {
		return ""
	}

	candidates := []string{
		relPath + ".py",
		filepath.ToSlash(filepath.Join(relPath, "__init__.py")),
	}
	for _, candidate := range candidates {
		if shouldIgnoreUploadBoundaryDependency(candidate) {
			continue
		}
		if uploadBoundaryFileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate))) {
			return normalizeUploadBoundaryPath(candidate)
		}
	}

	return ""
}

func discoverDirectRepoPathDependencies(repoRoot string, source string) []string {
	if strings.TrimSpace(source) == "" {
		return nil
	}

	rootSymbols := make(map[string]struct{}, len(defaultRepoRootReferenceVariable))
	for _, symbol := range defaultRepoRootReferenceVariable {
		rootSymbols[symbol] = struct{}{}
	}
	for _, match := range repoRootAssignmentPattern.FindAllStringSubmatch(source, -1) {
		rootSymbols[strings.TrimSpace(match[1])] = struct{}{}
	}

	for changed := true; changed; {
		changed = false
		for _, match := range repoRootAliasAssignmentPattern.FindAllStringSubmatch(source, -1) {
			target := strings.TrimSpace(match[1])
			sourceSymbol := strings.TrimSpace(match[2])
			if target == "" || sourceSymbol == "" {
				continue
			}
			if _, ok := rootSymbols[sourceSymbol]; !ok {
				continue
			}
			if _, ok := rootSymbols[target]; ok {
				continue
			}
			rootSymbols[target] = struct{}{}
			changed = true
		}
	}

	dependencies := make([]string, 0, 4)
	for _, match := range repoRootPathReferencePattern.FindAllStringSubmatch(source, -1) {
		baseSymbol := strings.TrimSpace(match[1])
		if _, ok := rootSymbols[baseSymbol]; !ok {
			continue
		}

		segments := extractRepoPathReferenceSegments(match[2])
		if len(segments) == 0 {
			continue
		}

		relativePath := normalizeUploadBoundaryPath(filepath.Join(segments...))
		if shouldIgnoreUploadBoundaryDependency(relativePath) {
			continue
		}
		if uploadBoundaryFileExists(filepath.Join(repoRoot, filepath.FromSlash(relativePath))) {
			dependencies = append(dependencies, relativePath)
		}
	}

	return dedupeUploadBoundaryPaths(dependencies)
}

func extractRepoPathReferenceSegments(raw string) []string {
	matches := repoRootPathReferencePartPattern.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	segments := make([]string, 0, len(matches))
	for _, match := range matches {
		candidate := strings.TrimSpace(match[1])
		if candidate == "" {
			candidate = strings.TrimSpace(match[2])
		}
		if candidate == "" {
			continue
		}
		segments = append(segments, candidate)
	}
	return segments
}

func uploadBoundaryFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func shouldIgnoreUploadBoundaryDependency(path string) bool {
	normalized := normalizeUploadBoundaryPath(path)
	if normalized == "" {
		return true
	}
	if strings.HasPrefix(normalized, ".") {
		return true
	}
	if strings.HasPrefix(normalized, "..") {
		return true
	}
	if _, ok := supportedBoundaryModelExtensions[strings.ToLower(filepath.Ext(normalized))]; ok {
		return true
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

func dedupeUploadBoundaryPaths(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeUploadBoundaryPath(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}
