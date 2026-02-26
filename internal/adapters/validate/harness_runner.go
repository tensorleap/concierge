package validate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

const (
	// HarnessEnableEnvVar controls whether runtime harness execution is enabled.
	HarnessEnableEnvVar = "CONCIERGE_ENABLE_HARNESS"
	harnessEnabledValue = "1"
)

const defaultHarnessTimeout = 120 * time.Second

type harnessCommandRunner func(ctx context.Context, dir, command string, args ...string) ([]byte, []byte, error)

// HarnessRunResult captures parsed output from a harness invocation.
type HarnessRunResult struct {
	Enabled bool
	Events  []HarnessEvent
	Issues  []core.Issue
}

// HarnessRunner invokes an optional Python harness and parses NDJSON output.
type HarnessRunner struct {
	timeout    time.Duration
	scriptPath string
	getEnv     func(string) string
	lookPath   func(string) (string, error)
	runCommand harnessCommandRunner
}

// NewHarnessRunner creates a harness runner with default command wiring.
func NewHarnessRunner() *HarnessRunner {
	return &HarnessRunner{
		timeout:    defaultHarnessTimeout,
		scriptPath: filepath.Join("scripts", "harness_stub.py"),
		getEnv:     os.Getenv,
		lookPath:   exec.LookPath,
		runCommand: runHarnessCommand,
	}
}

// Run executes the harness when enabled and parses its NDJSON output.
func (r *HarnessRunner) Run(ctx context.Context, snapshot core.WorkspaceSnapshot) (HarnessRunResult, error) {
	r.ensureDefaults()

	if strings.TrimSpace(r.getEnv(HarnessEnableEnvVar)) != harnessEnabledValue {
		return HarnessRunResult{Enabled: false}, nil
	}

	scriptPath, err := r.resolveScriptPath()
	if err != nil {
		return HarnessRunResult{}, err
	}
	pythonPath, err := r.resolvePythonPath()
	if err != nil {
		return HarnessRunResult{}, err
	}

	runDir := strings.TrimSpace(snapshot.Repository.Root)
	if runDir == "" {
		runDir = "."
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	stdout, stderr, err := r.runCommand(runCtx, runDir, pythonPath, scriptPath, "--repo-root", runDir)
	if err != nil {
		errWithStderr := err
		if stderrText := strings.TrimSpace(string(stderr)); stderrText != "" {
			errWithStderr = fmt.Errorf("%w (stderr: %s)", err, stderrText)
		}
		return HarnessRunResult{}, core.WrapError(core.KindUnknown, "validate.harness.run", errWithStderr)
	}

	events, issues, err := ParseHarnessEvents(stdout)
	if err != nil {
		return HarnessRunResult{}, err
	}

	return HarnessRunResult{
		Enabled: true,
		Events:  events,
		Issues:  issues,
	}, nil
}

func (r *HarnessRunner) ensureDefaults() {
	if r.timeout <= 0 {
		r.timeout = defaultHarnessTimeout
	}
	if strings.TrimSpace(r.scriptPath) == "" {
		r.scriptPath = filepath.Join("scripts", "harness_stub.py")
	}
	if r.getEnv == nil {
		r.getEnv = os.Getenv
	}
	if r.lookPath == nil {
		r.lookPath = exec.LookPath
	}
	if r.runCommand == nil {
		r.runCommand = runHarnessCommand
	}
}

func (r *HarnessRunner) resolveScriptPath() (string, error) {
	scriptPath, err := filepath.Abs(strings.TrimSpace(r.scriptPath))
	if err != nil {
		return "", core.WrapError(core.KindUnknown, "validate.harness.script_abs", err)
	}

	if _, err := os.Stat(scriptPath); err != nil {
		return "", core.WrapError(core.KindUnknown, "validate.harness.script_stat", err)
	}

	return scriptPath, nil
}

func (r *HarnessRunner) resolvePythonPath() (string, error) {
	pythonPath, err := r.lookPath("python3")
	if err == nil {
		return pythonPath, nil
	}

	fallbackPath, fallbackErr := r.lookPath("python")
	if fallbackErr == nil {
		return fallbackPath, nil
	}

	combined := errors.Join(err, fallbackErr)
	return "", core.WrapError(core.KindUnknown, "validate.harness.python_lookup", combined)
}

func runHarnessCommand(ctx context.Context, dir, command string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
