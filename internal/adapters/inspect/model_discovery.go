package inspect

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	modelCandidateSourceRepoSearch = "repo_search"
)

var (
	modelStringLiteralPattern = regexp.MustCompile(`['\"]([^'\"\n]+\.[A-Za-z0-9]+)['\"]`)

	supportedModelExtensions = map[string]struct{}{
		".onnx": {},
		".h5":   {},
	}

	modelLikeExtensions = map[string]struct{}{
		".onnx":   {},
		".h5":     {},
		".hdf5":   {},
		".keras":  {},
		".pb":     {},
		".pt":     {},
		".pth":    {},
		".ckpt":   {},
		".tflite": {},
	}
)

type modelCandidateCollector struct {
	repoRoot  string
	byPathKey map[string]*collectedModelCandidate
}

type collectedModelCandidate struct {
	path    string
	sources map[string]struct{}
}

type modelCandidateEvaluation struct {
	Candidate       core.ModelCandidate
	AbsolutePath    string
	DisplayPath     string
	SupportedFormat bool
	InsideRepo      bool
	Exists          bool
}

func discoverModelCandidates(repoRoot string, contract *leapYAMLContract, contracts *core.IntegrationContracts) ([]core.ModelCandidate, error) {
	fromRepo, err := discoverModelCandidatesFromRepoSearch(repoRoot)
	if err != nil {
		return nil, err
	}
	_ = contract
	_ = contracts
	return fromRepo, nil
}

func (c *modelCandidateCollector) add(path string, source string) {
	normalizedPath := normalizeModelCandidatePath(c.repoRoot, path)
	if normalizedPath == "" {
		return
	}
	normalizedSource := strings.TrimSpace(source)
	if normalizedSource == "" {
		normalizedSource = "unknown"
	}

	key := strings.ToLower(normalizedPath)
	aggregate, exists := c.byPathKey[key]
	if !exists {
		aggregate = &collectedModelCandidate{
			path:    normalizedPath,
			sources: map[string]struct{}{normalizedSource: {}},
		}
		c.byPathKey[key] = aggregate
		return
	}
	aggregate.sources[normalizedSource] = struct{}{}
}

func (c *modelCandidateCollector) list() []core.ModelCandidate {
	if len(c.byPathKey) == 0 {
		return nil
	}

	candidates := make([]core.ModelCandidate, 0, len(c.byPathKey))
	for _, aggregate := range c.byPathKey {
		sources := make([]string, 0, len(aggregate.sources))
		for source := range aggregate.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		candidates = append(candidates, core.ModelCandidate{
			Path:   aggregate.path,
			Source: strings.Join(sources, ","),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(candidates[i].Path))
		right := strings.ToLower(strings.TrimSpace(candidates[j].Path))
		if left != right {
			return left < right
		}
		return candidates[i].Source < candidates[j].Source
	})

	return candidates
}

func normalizeModelCandidatePath(repoRoot string, path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}

	candidatePath := filepath.FromSlash(trimmed)
	if !filepath.IsAbs(candidatePath) {
		candidatePath = filepath.Join(repoRoot, candidatePath)
	}
	candidatePath = filepath.Clean(candidatePath)

	if isPathWithinRepo(repoRoot, candidatePath) {
		rel, err := filepath.Rel(repoRoot, candidatePath)
		if err == nil {
			return filepath.ToSlash(filepath.Clean(rel))
		}
	}

	return filepath.ToSlash(candidatePath)
}

func discoverModelCandidatesFromLoadModelDecorators(repoRoot string, contract *leapYAMLContract, contracts *core.IntegrationContracts) ([]core.ModelCandidate, error) {
	if contract == nil || contracts == nil || len(contracts.LoadModelFunctions) == 0 {
		return nil, nil
	}

	entryFile := strings.TrimSpace(contract.EntryFile)
	if entryFile == "" {
		return nil, nil
	}

	entryAbsPath := entryFile
	if !filepath.IsAbs(entryAbsPath) {
		entryAbsPath = filepath.Join(repoRoot, filepath.FromSlash(entryFile))
	}
	entryAbsPath = filepath.Clean(entryAbsPath)
	if !isPathWithinRepo(repoRoot, entryAbsPath) {
		return nil, nil
	}

	info, err := os.Stat(entryAbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.model_discovery.entry_stat", err)
	}
	if info.IsDir() {
		return nil, nil
	}

	contents, err := os.ReadFile(entryAbsPath)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.model_discovery.entry_read", err)
	}

	return discoverModelCandidatesFromPythonSource(string(contents), contracts.LoadModelFunctions), nil
}

func discoverModelCandidatesFromPythonSource(source string, loadModelFunctions []string) []core.ModelCandidate {
	if strings.TrimSpace(source) == "" || len(loadModelFunctions) == 0 {
		return nil
	}

	targetFunctions := make(map[string]struct{}, len(loadModelFunctions))
	for _, functionName := range loadModelFunctions {
		trimmed := strings.TrimSpace(functionName)
		if trimmed == "" {
			continue
		}
		targetFunctions[trimmed] = struct{}{}
	}
	if len(targetFunctions) == 0 {
		return nil
	}

	lines := strings.Split(source, "\n")
	candidates := make([]core.ModelCandidate, 0, 4)
	for lineIndex, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(trimmed, "def") {
			continue
		}
		functionName, ok := extractFunctionName(trimmed)
		if !ok {
			continue
		}
		if _, exists := targetFunctions[functionName]; !exists {
			continue
		}

		defIndent := indentationLevel(rawLine)
		for i := lineIndex + 1; i < len(lines); i++ {
			line := lines[i]
			lineTrimmed := strings.TrimSpace(line)
			if lineTrimmed == "" || strings.HasPrefix(lineTrimmed, "#") {
				continue
			}
			if indentationLevel(line) <= defIndent {
				break
			}

			modelPaths := extractModelLikeStringLiterals(lineTrimmed)
			for _, modelPath := range modelPaths {
				candidates = append(candidates, core.ModelCandidate{
					Path:   modelPath,
					Source: "load_model." + functionName,
				})
			}
		}
	}

	return candidates
}

func extractModelLikeStringLiterals(line string) []string {
	matches := modelStringLiteralPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}

	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		if value == "" || strings.Contains(value, "{") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(value))
		if _, ok := modelLikeExtensions[ext]; !ok {
			continue
		}
		values = append(values, filepath.ToSlash(filepath.Clean(filepath.FromSlash(value))))
	}
	return values
}

func discoverModelCandidatesFromRepoSearch(repoRoot string) ([]core.ModelCandidate, error) {
	candidates := make([]core.ModelCandidate, 0, 2)
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !isSupportedModelExtension(ext) {
			return nil
		}

		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		candidates = append(candidates, core.ModelCandidate{
			Path:   filepath.ToSlash(filepath.Clean(rel)),
			Source: modelCandidateSourceRepoSearch,
		})
		return nil
	})
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.baseline.model_discovery.repo_search", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].Path) < strings.ToLower(candidates[j].Path)
	})
	return candidates, nil
}

func evaluateModelCandidate(repoRoot string, candidate core.ModelCandidate) (modelCandidateEvaluation, error) {
	displayPath := strings.TrimSpace(candidate.Path)
	if displayPath == "" {
		return modelCandidateEvaluation{}, nil
	}

	absolutePath := filepath.FromSlash(displayPath)
	if !filepath.IsAbs(absolutePath) {
		absolutePath = filepath.Join(repoRoot, absolutePath)
	}
	absolutePath = filepath.Clean(absolutePath)

	evaluation := modelCandidateEvaluation{
		Candidate:       candidate,
		AbsolutePath:    absolutePath,
		DisplayPath:     displayPath,
		SupportedFormat: isSupportedModelExtension(strings.ToLower(filepath.Ext(displayPath))),
	}

	evaluation.InsideRepo = isPathWithinRepo(repoRoot, absolutePath)
	if !evaluation.InsideRepo {
		return evaluation, nil
	}

	rel, err := filepath.Rel(repoRoot, absolutePath)
	if err != nil {
		return modelCandidateEvaluation{}, core.WrapError(core.KindUnknown, "inspect.baseline.model_discovery.rel", err)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	evaluation.DisplayPath = rel

	if info, statErr := os.Stat(absolutePath); statErr == nil {
		evaluation.Exists = !info.IsDir()
	} else if !os.IsNotExist(statErr) {
		return modelCandidateEvaluation{}, core.WrapError(core.KindUnknown, "inspect.baseline.model_discovery.stat", statErr)
	}

	return evaluation, nil
}

func isSupportedModelExtension(ext string) bool {
	_, ok := supportedModelExtensions[strings.ToLower(strings.TrimSpace(ext))]
	return ok
}
