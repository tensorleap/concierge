package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type poetryRuntimeCommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)

const (
	poetryRuntimeProbeTimeout         = 5 * time.Second
	poetryRuntimeFallbackProbeTimeout = 10 * time.Second
)

const codeLoaderCapabilityProbeScript = `
import json

result = {
    "probeSucceeded": True,
    "version": "",
    "supportsGuideLocalStatusTable": False,
    "supportsCheckDataset": False,
}

try:
    try:
        from importlib import metadata as importlib_metadata
    except ImportError:
        import importlib_metadata  # type: ignore
    for package_name in ("code-loader", "code_loader"):
        try:
            result["version"] = importlib_metadata.version(package_name)
            break
        except Exception:
            continue
except Exception:
    pass

try:
    from code_loader import LeapLoader
    result["supportsCheckDataset"] = hasattr(LeapLoader, "check_dataset")
except Exception:
    result["probeSucceeded"] = False

try:
    from code_loader.inner_leap_binder import leapbinder_decorators as decorators
    result["supportsGuideLocalStatusTable"] = hasattr(decorators, "tensorleap_status_table")
except Exception:
    result["probeSucceeded"] = False

print(json.dumps(result))
`

var (
	poetryDependencySectionPattern = regexp.MustCompile(`^\[tool\.poetry(\.group\.[^.]+)?\.dependencies\]\s*$`)
	codeLoaderDependencyPattern   = regexp.MustCompile(`^(code-loader|code_loader)\s*=`)
)

type codeLoaderCapabilityProbe struct {
	ProbeSucceeded                bool   `json:"probeSucceeded"`
	Version                       string `json:"version"`
	SupportsGuideLocalStatusTable bool   `json:"supportsGuideLocalStatusTable"`
	SupportsCheckDataset          bool   `json:"supportsCheckDataset"`
}

// PoetryRuntimeResolution captures a resolved runtime profile plus any confirmation-worthy signals.
type PoetryRuntimeResolution struct {
	Profile           *core.LocalRuntimeProfile
	SuspiciousReasons []string
}

// PoetryRuntimeResolver resolves one Poetry runtime profile for the selected repo root.
type PoetryRuntimeResolver struct {
	runCommand poetryRuntimeCommandRunner
}

// NewPoetryRuntimeResolver creates a resolver backed by local Poetry commands.
func NewPoetryRuntimeResolver() *PoetryRuntimeResolver {
	return &PoetryRuntimeResolver{runCommand: runPoetryRuntimeCommand}
}

// Resolve builds a LocalRuntimeProfile when an existing Poetry environment can be identified.
func (r *PoetryRuntimeResolver) Resolve(
	ctx context.Context,
	repoRoot string,
	snapshot core.WorkspaceSnapshot,
	previous *core.LocalRuntimeProfile,
) (PoetryRuntimeResolution, error) {
	if r == nil {
		r = NewPoetryRuntimeResolver()
	}
	if r.runCommand == nil {
		r.runCommand = runPoetryRuntimeCommand
	}
	if !snapshot.Runtime.SupportedProject || !snapshot.Runtime.PoetryFound {
		return PoetryRuntimeResolution{}, nil
	}

	interpreterPath, err := resolvePoetryInterpreterPath(ctx, r.runCommand, repoRoot)
	if err != nil || strings.TrimSpace(interpreterPath) == "" {
		return PoetryRuntimeResolution{}, nil
	}
	interpreterPath = normalizeRuntimePath(interpreterPath)

	pythonVersion, versionErr := runPoetryCommandText(ctx, r.runCommand, repoRoot, interpreterPath, "--version")
	if versionErr != nil {
		pythonVersion = ""
	}

	profile := &core.LocalRuntimeProfile{
		Kind:             "poetry",
		PoetryExecutable: strings.TrimSpace(snapshot.Runtime.PoetryExecutable),
		PoetryVersion:    strings.TrimSpace(snapshot.Runtime.PoetryVersion),
		InterpreterPath:  interpreterPath,
		PythonVersion:    strings.TrimSpace(pythonVersion),
		ConfirmationMode: "auto",
		Fingerprint: core.RuntimeProfileFingerprint{
			ProjectRoot:     filepath.Clean(repoRoot),
			PyProjectHash:   strings.TrimSpace(snapshot.FileHashes["pyproject.toml"]),
			PoetryLockHash:  strings.TrimSpace(snapshot.FileHashes["poetry.lock"]),
			InterpreterPath: interpreterPath,
			PythonVersion:   strings.TrimSpace(pythonVersion),
		},
	}
	codeLoaderDeclared, err := detectProjectCodeLoaderDeclaration(repoRoot)
	if err != nil {
		return PoetryRuntimeResolution{}, core.WrapError(core.KindUnknown, "inspect.runtime_profile.code_loader_declared", err)
	}
	profile.CodeLoaderDeclaredInProject = codeLoaderDeclared
	profile.DependenciesReady = runPoetryReadinessCheck(ctx, r.runCommand, repoRoot, "poetry", "check")
	profile.CodeLoaderReady = runPoetryReadinessCheck(
		ctx,
		r.runCommand,
		repoRoot,
		interpreterPath,
		"-c",
		"import code_loader",
	)
	if profile.CodeLoaderReady {
		profile.CodeLoader = probeCodeLoaderCapabilities(ctx, r.runCommand, repoRoot, interpreterPath)
	}

	return PoetryRuntimeResolution{
		Profile:           profile,
		SuspiciousReasons: suspiciousRuntimeReasons(snapshot, previous, profile),
	}, nil
}

func suspiciousRuntimeReasons(
	snapshot core.WorkspaceSnapshot,
	previous *core.LocalRuntimeProfile,
	current *core.LocalRuntimeProfile,
) []string {
	if current == nil {
		return nil
	}

	reasons := make([]string, 0, 3)
	if ambient := strings.TrimSpace(snapshot.Runtime.AmbientVirtualEnv); ambient != "" &&
		!runtimePathMatchesPrefix(current.InterpreterPath, ambient) {
		reasons = append(reasons, "the active VIRTUAL_ENV does not match Poetry's selected environment")
	}
	if ambient := strings.TrimSpace(snapshot.Runtime.AmbientCondaPrefix); ambient != "" &&
		!runtimePathMatchesPrefix(current.InterpreterPath, ambient) {
		reasons = append(reasons, "the active CONDA_PREFIX does not match Poetry's selected environment")
	}
	if previous != nil &&
		previous.Fingerprint.PyProjectHash == current.Fingerprint.PyProjectHash &&
		previous.Fingerprint.PoetryLockHash == current.Fingerprint.PoetryLockHash &&
		previous.InterpreterPath != "" &&
		previous.InterpreterPath != current.InterpreterPath {
		reasons = append(reasons, "Poetry resolved a different interpreter even though pyproject.toml and poetry.lock did not change")
	}
	return reasons
}

func runtimePathMatchesPrefix(interpreterPath string, prefix string) bool {
	normalizedInterpreter := normalizeRuntimePath(interpreterPath)
	normalizedPrefix := normalizeRuntimePath(prefix)
	return normalizedInterpreter != "" && normalizedPrefix != "" && strings.HasPrefix(normalizedInterpreter, normalizedPrefix)
}

func normalizeRuntimePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

func cloneRuntimeProfile(profile *core.LocalRuntimeProfile) *core.LocalRuntimeProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.Fingerprint = profile.Fingerprint
	return &cloned
}

func detectProjectCodeLoaderDeclaration(repoRoot string) (bool, error) {
	pyprojectPath := filepath.Join(strings.TrimSpace(repoRoot), "pyproject.toml")
	raw, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return false, fmt.Errorf("read pyproject.toml: %w", err)
	}

	inDependenciesSection := false
	for _, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDependenciesSection = poetryDependencySectionPattern.MatchString(line)
			continue
		}
		if !inDependenciesSection {
			continue
		}
		if codeLoaderDependencyPattern.MatchString(line) {
			return true, nil
		}
	}

	return false, nil
}

func runPoetryRuntimeCommand(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func runPoetryCommandText(
	ctx context.Context,
	runner poetryRuntimeCommandRunner,
	dir string,
	name string,
	args ...string,
) (string, error) {
	return runCommandTextWithTimeout(ctx, runner, poetryRuntimeProbeTimeout, dir, name, args...)
}

func runCommandTextWithTimeout(
	ctx context.Context,
	runner poetryRuntimeCommandRunner,
	timeout time.Duration,
	dir string,
	name string,
	args ...string,
) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, err := runner(commandCtx, dir, name, args...)
	if err != nil {
		return "", err
	}
	if text := strings.TrimSpace(string(stdout)); text != "" {
		return text, nil
	}
	return strings.TrimSpace(string(stderr)), nil
}

func resolvePoetryInterpreterPath(
	ctx context.Context,
	runner poetryRuntimeCommandRunner,
	dir string,
) (string, error) {
	interpreterPath, err := runCommandTextWithTimeout(
		ctx,
		runner,
		poetryRuntimeProbeTimeout,
		dir,
		"poetry",
		"env",
		"info",
		"--executable",
	)
	if err == nil && isUsableInterpreterPath(interpreterPath) {
		return interpreterPath, nil
	}

	fallbackPath, fallbackErr := runCommandTextWithTimeout(
		ctx,
		runner,
		poetryRuntimeFallbackProbeTimeout,
		dir,
		"poetry",
		"run",
		"python",
		"-c",
		"import sys; print(sys.executable)",
	)
	if fallbackErr != nil {
		if err != nil {
			return "", err
		}
		return "", fallbackErr
	}
	if !isUsableInterpreterPath(fallbackPath) {
		if err != nil {
			return "", err
		}
		return "", nil
	}
	return fallbackPath, nil
}

func isUsableInterpreterPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	return !strings.EqualFold(trimmed, "NA")
}

func probeCodeLoaderCapabilities(
	ctx context.Context,
	runner poetryRuntimeCommandRunner,
	dir string,
	interpreterPath string,
) core.CodeLoaderCapabilityState {
	output, err := runPoetryCommandText(
		ctx,
		runner,
		dir,
		interpreterPath,
		"-c",
		codeLoaderCapabilityProbeScript,
	)
	if err != nil || strings.TrimSpace(output) == "" {
		return core.CodeLoaderCapabilityState{}
	}

	var probe codeLoaderCapabilityProbe
	if err := json.Unmarshal([]byte(output), &probe); err != nil {
		return core.CodeLoaderCapabilityState{}
	}

	return core.CodeLoaderCapabilityState{
		ProbeSucceeded:                probe.ProbeSucceeded,
		Version:                       strings.TrimSpace(probe.Version),
		SupportsGuideLocalStatusTable: probe.SupportsGuideLocalStatusTable,
		SupportsCheckDataset:          probe.SupportsCheckDataset,
	}
}

func runPoetryReadinessCheck(
	ctx context.Context,
	runner poetryRuntimeCommandRunner,
	dir string,
	name string,
	args ...string,
) bool {
	commandCtx, cancel := context.WithTimeout(ctx, poetryRuntimeProbeTimeout)
	defer cancel()

	_, _, err := runner(commandCtx, dir, name, args...)
	return err == nil
}
