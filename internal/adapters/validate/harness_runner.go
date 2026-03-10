package validate

import (
	"context"
	"os"
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

// HarnessRunResult captures parsed output from a harness invocation.
type HarnessRunResult struct {
	Enabled  bool
	Events   []HarnessEvent
	Issues   []core.Issue
	Evidence []core.EvidenceItem
}

// HarnessRunner invokes an optional Python harness and parses NDJSON output.
type HarnessRunner struct {
	timeout       time.Duration
	scriptPath    string
	getEnv        func(string) string
	runtimeRunner *PythonRuntimeRunner
}

// NewHarnessRunner creates a harness runner with default command wiring.
func NewHarnessRunner() *HarnessRunner {
	return &HarnessRunner{
		timeout:       defaultHarnessTimeout,
		scriptPath:    filepath.Join("scripts", "harness_stub.py"),
		getEnv:        os.Getenv,
		runtimeRunner: NewPythonRuntimeRunner(),
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
	runDir := strings.TrimSpace(snapshot.Repository.Root)
	if runDir == "" {
		runDir = "."
	}

	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	commandResult, err := r.runtimeRunner.RunPython(runCtx, snapshot, scriptPath, "--repo-root", runDir)
	if err != nil {
		return HarnessRunResult{}, core.WrapError(core.KindUnknown, "validate.harness.run", err)
	}

	events, issues, err := ParseHarnessEvents([]byte(commandResult.Stdout))
	if err != nil {
		return HarnessRunResult{}, err
	}

	return HarnessRunResult{
		Enabled: true,
		Events:  events,
		Issues:  issues,
		Evidence: []core.EvidenceItem{
			{Name: "runtime.command", Value: commandResult.Command},
			{Name: "runtime.stdout", Value: commandResult.Stdout},
			{Name: "runtime.stderr", Value: commandResult.Stderr},
			{Name: "runtime.poetry_version", Value: strings.TrimSpace(snapshot.Runtime.PoetryVersion)},
			{Name: "runtime.interpreter_path", Value: strings.TrimSpace(snapshot.Runtime.ResolvedInterpreter)},
			{Name: "runtime.python_version", Value: strings.TrimSpace(snapshot.Runtime.ResolvedPythonVersion)},
		},
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
	if r.runtimeRunner == nil {
		r.runtimeRunner = NewPythonRuntimeRunner()
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
