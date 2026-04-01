package core

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
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

	required = append(required, discoverDirectRepoPathDependencies(string(source), repoRoot)...)
	return dedupeUploadBoundaryPaths(required)
}

// DirectRepoUploadBoundaryDependencies returns only the direct repo-local file
// references rooted from leap_integration.py itself. It intentionally excludes
// the static leap.yaml/requirements baseline so inspectors can surface the
// new boundary gap without reclassifying older fixtures around legacy static
// upload-rule mismatches.
func DirectRepoUploadBoundaryDependencies(repoRoot string, entryFile string) []string {
	entryPath := resolveUploadBoundaryEntryPath(repoRoot, entryFile)
	if entryPath == "" {
		return nil
	}

	source, err := os.ReadFile(entryPath)
	if err != nil {
		return nil
	}

	return discoverDirectRepoPathDependencies(string(source), repoRoot)
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

func discoverDirectRepoPathDependencies(source string, repoRoot string) []string {
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
