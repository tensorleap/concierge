package inspect

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

type poetryRuntimeCommandRunner func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)

const poetryRuntimeProbeTimeout = 3 * time.Second

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

	interpreterPath, err := runPoetryCommandText(ctx, r.runCommand, repoRoot, "poetry", "env", "info", "--executable")
	if err != nil || strings.TrimSpace(interpreterPath) == "" {
		return PoetryRuntimeResolution{}, nil
	}
	interpreterPath = normalizeRuntimePath(interpreterPath)

	pythonVersion, versionErr := runPoetryCommandText(ctx, r.runCommand, repoRoot, "poetry", "run", "python", "--version")
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
	profile.DependenciesReady = runPoetryReadinessCheck(ctx, r.runCommand, repoRoot, "poetry", "check")
	profile.CodeLoaderReady = runPoetryReadinessCheck(
		ctx,
		r.runCommand,
		repoRoot,
		"poetry",
		"run",
		"python",
		"-c",
		"import code_loader",
	)

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
	commandCtx, cancel := context.WithTimeout(ctx, poetryRuntimeProbeTimeout)
	defer cancel()

	stdout, stderr, err := runner(commandCtx, dir, name, args...)
	text := strings.TrimSpace(strings.TrimSpace(string(stdout)) + "\n" + strings.TrimSpace(string(stderr)))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
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
