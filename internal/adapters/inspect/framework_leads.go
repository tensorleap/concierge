package inspect

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	defaultFrameworkLeadTopN           = 20
	defaultFrameworkLeadMaxOccurrences = 5
)

var (
	frameworkLeadSkipDirs = map[string]struct{}{
		".git":          {},
		".venv":         {},
		"venv":          {},
		"__pycache__":   {},
		"build":         {},
		"dist":          {},
		".mypy_cache":   {},
		".pytest_cache": {},
	}
)

type compiledFrameworkSignal struct {
	definition frameworkLeadSignalDefinition
	regex      *regexp.Regexp
}

type frameworkLeadExtractor struct {
	topN           int
	maxOccurrences int
}

func newFrameworkLeadExtractor() *frameworkLeadExtractor {
	return &frameworkLeadExtractor{
		topN:           defaultFrameworkLeadTopN,
		maxOccurrences: defaultFrameworkLeadMaxOccurrences,
	}
}

func (e *frameworkLeadExtractor) Extract(repoRoot string) (core.InputGTLeadPack, string, error) {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		return core.InputGTLeadPack{}, "", core.NewError(
			core.KindUnknown,
			"inspect.framework_leads.repo_root",
			"repository root is empty",
		)
	}

	compiledSignals, err := compileFrameworkSignals()
	if err != nil {
		return core.InputGTLeadPack{}, "", err
	}

	pythonFiles, err := collectPythonFiles(root)
	if err != nil {
		return core.InputGTLeadPack{}, "", err
	}
	sort.Strings(pythonFiles)

	leadFiles := make([]core.InputGTLeadFile, 0, len(pythonFiles))
	signalHitCount := 0
	signalFrameworkScores := core.InputGTFrameworkScore{}
	for _, filePath := range pythonFiles {
		relativePath, relErr := filepath.Rel(root, filePath)
		if relErr != nil {
			return core.InputGTLeadPack{}, "", core.WrapError(core.KindUnknown, "inspect.framework_leads.relative_path", relErr)
		}
		leadFile, hits, fileErr := e.scanFile(filePath, filepath.ToSlash(filepath.Clean(relativePath)), compiledSignals)
		if fileErr != nil {
			return core.InputGTLeadPack{}, "", fileErr
		}
		if len(leadFile.SignalHits) == 0 {
			continue
		}
		leadFiles = append(leadFiles, leadFile)
		signalHitCount += hits
		signalFrameworkScores.PyTorch += leadFile.FrameworkScores.PyTorch
		signalFrameworkScores.TensorFlow += leadFile.FrameworkScores.TensorFlow
	}

	artifactScores, artifactEvidence, err := detectFrameworkArtifacts(root)
	if err != nil {
		return core.InputGTLeadPack{}, "", err
	}

	frameworkScores := core.InputGTFrameworkScore{
		PyTorch:    round2(signalFrameworkScores.PyTorch + artifactScores.PyTorch),
		TensorFlow: round2(signalFrameworkScores.TensorFlow + artifactScores.TensorFlow),
	}
	candidate, confidence := classifyFramework(frameworkScores.PyTorch, frameworkScores.TensorFlow)

	sort.SliceStable(leadFiles, func(i, j int) bool {
		if leadFiles[i].Score != leadFiles[j].Score {
			return leadFiles[i].Score > leadFiles[j].Score
		}
		return strings.ToLower(leadFiles[i].Path) < strings.ToLower(leadFiles[j].Path)
	})

	topFiles := leadFiles
	if len(topFiles) > e.topN {
		topFiles = append([]core.InputGTLeadFile(nil), leadFiles[:e.topN]...)
	} else {
		topFiles = append([]core.InputGTLeadFile(nil), leadFiles...)
	}

	leadPack := core.InputGTLeadPack{
		SchemaVersion:      frameworkLeadSchemaVersion,
		MethodVersion:      frameworkLeadMethodVersion,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		RepoPath:           root,
		PythonFilesScanned: len(pythonFiles),
		FrameworkDetection: core.InputGTFrameworkDetection{
			Candidate:  candidate,
			Confidence: confidence,
			Scores:     frameworkScores,
			Components: core.InputGTFrameworkComponents{
				SignalScores: core.InputGTFrameworkScore{
					PyTorch:    round2(signalFrameworkScores.PyTorch),
					TensorFlow: round2(signalFrameworkScores.TensorFlow),
				},
				ArtifactScores: artifactScores,
			},
			Evidence: artifactEvidence,
		},
		Signals:        frameworkSignalsForReport(),
		Files:          topFiles,
		FilesWithHits:  len(leadFiles),
		SignalHitCount: signalHitCount,
	}

	summary := buildFrameworkLeadSummary(leadPack)
	return leadPack, summary, nil
}

func compileFrameworkSignals() ([]compiledFrameworkSignal, error) {
	compiled := make([]compiledFrameworkSignal, 0, len(frameworkLeadSignalDefinitions))
	for _, definition := range frameworkLeadSignalDefinitions {
		expression, err := regexp.Compile(definition.Pattern)
		if err != nil {
			return nil, core.WrapError(core.KindUnknown, "inspect.framework_leads.compile_signal", err)
		}
		compiled = append(compiled, compiledFrameworkSignal{definition: definition, regex: expression})
	}
	return compiled, nil
}

func collectPythonFiles(repoRoot string) ([]string, error) {
	files := make([]string, 0, 64)
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if _, skip := frameworkLeadSkipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".py") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "inspect.framework_leads.collect_python_files", err)
	}
	return files, nil
}

func (e *frameworkLeadExtractor) scanFile(
	absolutePath string,
	relativePath string,
	signals []compiledFrameworkSignal,
) (core.InputGTLeadFile, int, error) {
	raw, err := os.ReadFile(absolutePath)
	if err != nil {
		return core.InputGTLeadFile{}, 0, core.WrapError(core.KindUnknown, "inspect.framework_leads.read_file", err)
	}
	lines := strings.Split(string(raw), "\n")

	signalHits := make([]core.InputGTLeadSignalHit, 0, len(signals))
	totalScore := 0.0
	frameworkScores := core.InputGTFrameworkScore{}
	totalHits := 0

	for _, signal := range signals {
		occurrences := make([]core.InputGTLeadSignalOccurrence, 0, e.maxOccurrences)
		count := 0
		for lineNumber, line := range lines {
			if !signal.regex.MatchString(line) {
				continue
			}
			count++
			if len(occurrences) < e.maxOccurrences {
				occurrences = append(occurrences, core.InputGTLeadSignalOccurrence{
					Line:    lineNumber + 1,
					Snippet: clipFrameworkLeadSnippet(line),
				})
			}
		}
		if count == 0 {
			continue
		}

		cappedCount := count
		if cappedCount > 5 {
			cappedCount = 5
		}
		contribution := signal.definition.Weight * float64(cappedCount)
		totalScore += contribution
		totalHits += count

		switch signal.definition.Framework {
		case "pytorch":
			frameworkScores.PyTorch += contribution
		case "tensorflow":
			frameworkScores.TensorFlow += contribution
		}

		signalHits = append(signalHits, core.InputGTLeadSignalHit{
			SignalID:     signal.definition.ID,
			Framework:    signal.definition.Framework,
			Count:        count,
			Contribution: round2(contribution),
			Occurrences:  occurrences,
		})
	}

	sort.SliceStable(signalHits, func(i, j int) bool {
		if signalHits[i].Contribution != signalHits[j].Contribution {
			return signalHits[i].Contribution > signalHits[j].Contribution
		}
		return signalHits[i].SignalID < signalHits[j].SignalID
	})

	return core.InputGTLeadFile{
		Path:  relativePath,
		Score: round2(totalScore),
		FrameworkScores: core.InputGTFrameworkScore{
			PyTorch:    round2(frameworkScores.PyTorch),
			TensorFlow: round2(frameworkScores.TensorFlow),
		},
		SignalHits: signalHits,
	}, totalHits, nil
}

func detectFrameworkArtifacts(repoRoot string) (core.InputGTFrameworkScore, []core.InputGTFrameworkEvidence, error) {
	scores := core.InputGTFrameworkScore{}
	evidence := make([]core.InputGTFrameworkEvidence, 0, 32)

	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if _, skip := frameworkLeadSkipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}

		suffix := strings.ToLower(filepath.Ext(entry.Name()))
		for framework, weightMap := range frameworkArtifactSuffixWeights {
			weight, ok := weightMap[suffix]
			if !ok {
				continue
			}
			relativePath, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return relErr
			}
			switch framework {
			case "pytorch":
				scores.PyTorch += weight
			case "tensorflow":
				scores.TensorFlow += weight
			}
			evidence = append(evidence, core.InputGTFrameworkEvidence{
				Framework: framework,
				Type:      "model_artifact",
				Path:      filepath.ToSlash(filepath.Clean(relativePath)),
				Detail:    "suffix=" + suffix,
				Weight:    round2(weight),
			})
		}

		return nil
	})
	if err != nil {
		return core.InputGTFrameworkScore{}, nil, core.WrapError(core.KindUnknown, "inspect.framework_leads.detect_artifacts", err)
	}

	dependencyScores, dependencyEvidence, dependencyErr := detectFrameworkDependencies(repoRoot)
	if dependencyErr != nil {
		return core.InputGTFrameworkScore{}, nil, dependencyErr
	}

	scores.PyTorch += dependencyScores.PyTorch
	scores.TensorFlow += dependencyScores.TensorFlow
	evidence = append(evidence, dependencyEvidence...)

	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].Weight != evidence[j].Weight {
			return evidence[i].Weight > evidence[j].Weight
		}
		if evidence[i].Path != evidence[j].Path {
			return evidence[i].Path < evidence[j].Path
		}
		return evidence[i].Framework < evidence[j].Framework
	})

	return core.InputGTFrameworkScore{
		PyTorch:    round2(scores.PyTorch),
		TensorFlow: round2(scores.TensorFlow),
	}, evidence, nil
}

func detectFrameworkDependencies(repoRoot string) (core.InputGTFrameworkScore, []core.InputGTFrameworkEvidence, error) {
	scores := core.InputGTFrameworkScore{}
	evidence := make([]core.InputGTFrameworkEvidence, 0, 16)

	compiledPatterns := make(map[string][]*regexp.Regexp, len(frameworkDependencyPatterns))
	for framework, patterns := range frameworkDependencyPatterns {
		compiled := make([]*regexp.Regexp, 0, len(patterns))
		for _, pattern := range patterns {
			expression, err := regexp.Compile(pattern)
			if err != nil {
				return core.InputGTFrameworkScore{}, nil, core.WrapError(core.KindUnknown, "inspect.framework_leads.compile_dependency_pattern", err)
			}
			compiled = append(compiled, expression)
		}
		compiledPatterns[framework] = compiled
	}

	for _, relativePath := range frameworkDependencyFiles {
		absolutePath := filepath.Join(repoRoot, relativePath)
		info, err := os.Stat(absolutePath)
		if err != nil || info.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(absolutePath)
		if readErr != nil {
			return core.InputGTFrameworkScore{}, nil, core.WrapError(core.KindUnknown, "inspect.framework_leads.read_dependency_file", readErr)
		}
		content := strings.ToLower(string(raw))

		for framework, expressions := range compiledPatterns {
			matched := make([]string, 0, len(expressions))
			for _, expression := range expressions {
				if !expression.MatchString(content) {
					continue
				}
				matched = append(matched, normalizeDependencyPatternForDetail(expression.String()))
			}
			if len(matched) == 0 {
				continue
			}
			sort.Strings(matched)
			matched = uniqueStrings(matched)

			weight := 4.0 + float64(minInt(len(matched), 3))
			switch framework {
			case "pytorch":
				scores.PyTorch += weight
			case "tensorflow":
				scores.TensorFlow += weight
			}
			evidence = append(evidence, core.InputGTFrameworkEvidence{
				Framework: framework,
				Type:      "dependency_file",
				Path:      relativePath,
				Detail:    "matched=" + strings.Join(matched, ","),
				Weight:    round2(weight),
			})
		}
	}

	return scores, evidence, nil
}

func normalizeDependencyPatternForDetail(pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	trimmed = strings.TrimPrefix(trimmed, `\b`)
	trimmed = strings.TrimSuffix(trimmed, `\b`)
	trimmed = strings.ReplaceAll(trimmed, `\`, "")
	return trimmed
}

func classifyFramework(pytorchScore, tensorflowScore float64) (string, string) {
	if pytorchScore <= 0 && tensorflowScore <= 0 {
		return "unknown", "low"
	}

	top := pytorchScore
	low := tensorflowScore
	if tensorflowScore > pytorchScore {
		top = tensorflowScore
		low = pytorchScore
	}
	if low > 0 {
		ratio := low / top
		if ratio >= 0.6 {
			if low >= 12 && top >= 12 {
				return "mixed", "high"
			}
			return "mixed", "medium"
		}
	}

	framework := "pytorch"
	other := tensorflowScore
	major := pytorchScore
	if tensorflowScore > pytorchScore {
		framework = "tensorflow"
		other = pytorchScore
		major = tensorflowScore
	}

	if major >= 20 && (other == 0 || major >= (other*1.8)) {
		return framework, "high"
	}
	if major >= 8 {
		return framework, "medium"
	}
	return framework, "low"
}

func frameworkSignalsForReport() []core.InputGTLeadSignal {
	signals := make([]core.InputGTLeadSignal, 0, len(frameworkLeadSignalDefinitions))
	for _, signal := range frameworkLeadSignalDefinitions {
		signals = append(signals, core.InputGTLeadSignal{
			ID:          signal.ID,
			Framework:   signal.Framework,
			Description: signal.Description,
			Weight:      signal.Weight,
			Tier:        signal.Tier,
		})
	}
	return signals
}

func buildFrameworkLeadSummary(leadPack core.InputGTLeadPack) string {
	lines := []string{
		fmt.Sprintf("Method: %s", leadPack.MethodVersion),
		fmt.Sprintf("Repo: %s", leadPack.RepoPath),
		fmt.Sprintf("Python files scanned: %d", leadPack.PythonFilesScanned),
		fmt.Sprintf("Files with hits: %d", leadPack.FilesWithHits),
		fmt.Sprintf("Total signal hits: %d", leadPack.SignalHitCount),
		fmt.Sprintf(
			"Framework detection: %s (%s) [pytorch=%.2f, tensorflow=%.2f]",
			leadPack.FrameworkDetection.Candidate,
			leadPack.FrameworkDetection.Confidence,
			leadPack.FrameworkDetection.Scores.PyTorch,
			leadPack.FrameworkDetection.Scores.TensorFlow,
		),
		"",
		"Top lead files:",
	}

	for index, file := range leadPack.Files {
		lines = append(lines, fmt.Sprintf("%d. %s (score=%.2f)", index+1, file.Path, file.Score))
		topSignals := file.SignalHits
		if len(topSignals) > 3 {
			topSignals = topSignals[:3]
		}
		for _, signal := range topSignals {
			snippet := ""
			if len(signal.Occurrences) > 0 {
				snippet = strings.TrimSpace(signal.Occurrences[0].Snippet)
			}
			lines = append(lines, fmt.Sprintf(
				"   - %s[%s]: count=%d, contribution=%.2f, evidence=%q",
				signal.SignalID,
				signal.Framework,
				signal.Count,
				signal.Contribution,
				snippet,
			))
		}
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func clipFrameworkLeadSnippet(value string) string {
	const limit = 200
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit]
}

func round2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100.0
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	return unique
}
