package inspect

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectRequiredUploadBoundary(repoRoot string, contract *leapYAMLContract, status *core.IntegrationStatus) {
	if contract == nil || status == nil {
		return
	}

	required := core.RequiredUploadBoundaryPaths(repoRoot, contract.EntryFile)
	if len(required) == 0 {
		return
	}

	includePatterns := normalizeUploadBoundaryPatterns(contract.Include)
	excludePatterns := normalizeUploadBoundaryPatterns(contract.Exclude)

	missing := make([]string, 0, len(required))
	blocked := make([]string, 0, len(required))
	for _, path := range required {
		included := len(includePatterns) == 0 || matchesUploadBoundaryAny(path, includePatterns)
		excluded := matchesUploadBoundaryAny(path, excludePatterns)

		if len(includePatterns) > 0 && !included {
			missing = append(missing, path)
		}
		if !included && excluded {
			blocked = append(blocked, path)
		}
	}

	if len(missing) > 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLIncludeMissingRequiredFiles,
			Message:  fmt.Sprintf("leap.yaml include rules omit required repo files referenced by %s: %s", core.CanonicalIntegrationEntryFile, strings.Join(missing, ", ")),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeLeapYAML,
		})
	}
	if len(blocked) > 0 {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapYAMLExcludeBlocksRequiredFiles,
			Message:  fmt.Sprintf("leap.yaml exclude rules still block required repo files referenced by %s: %s", core.CanonicalIntegrationEntryFile, strings.Join(blocked, ", ")),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeLeapYAML,
		})
	}
}

func normalizeUploadBoundaryPatterns(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeUploadBoundaryPattern(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeUploadBoundaryPattern(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "." {
		return ""
	}
	return normalized
}

func matchesUploadBoundaryAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesUploadBoundaryPattern(path, pattern) {
			return true
		}
	}
	return false
}

func matchesUploadBoundaryPattern(path string, pattern string) bool {
	normalizedPath := normalizeUploadBoundaryPattern(path)
	normalizedPattern := normalizeUploadBoundaryPattern(pattern)
	if normalizedPath == "" || normalizedPattern == "" {
		return false
	}

	if strings.HasSuffix(normalizedPattern, "/**") {
		prefix := strings.TrimSuffix(normalizedPattern, "/**")
		return normalizedPath == prefix || strings.HasPrefix(normalizedPath, prefix+"/")
	}
	if strings.HasSuffix(normalizedPattern, "/*") {
		prefix := strings.TrimSuffix(normalizedPattern, "/*")
		if !strings.HasPrefix(normalizedPath, prefix+"/") {
			return false
		}
		remaining := strings.TrimPrefix(normalizedPath, prefix+"/")
		return remaining != "" && !strings.Contains(remaining, "/")
	}

	matched, err := pathMatch(normalizedPattern, normalizedPath)
	if err == nil && matched {
		return true
	}
	if !strings.ContainsAny(normalizedPattern, "*?[") {
		return normalizedPath == normalizedPattern
	}
	return false
}

func pathMatch(pattern, path string) (bool, error) {
	return filepath.Match(pattern, path)
}
