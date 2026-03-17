package inspect

import (
	"path/filepath"
	"sort"
	"strings"
)

func normalizeSelectedModelPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	byKey := make(map[string]string, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = trimmed
	}
	if len(byKey) == 0 {
		return nil
	}

	result := make([]string, 0, len(byKey))
	for _, value := range byKey {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(result[i])
		right := strings.ToLower(result[j])
		if left != right {
			return left < right
		}
		return result[i] < result[j]
	})
	return result
}
