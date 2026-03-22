package validate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHarnessRunnerCommandFailureBecomesIssue(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			return []byte("some stdout"), []byte("ImportError: No module named code_loader"), errors.New("exit status 1")
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: t.TempDir()},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("expected no Go error, got %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to be enabled")
	}
	if !hasIssueCode(result.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected harness failure issue, got %+v", result.Issues)
	}
	if !strings.Contains(result.Issues[0].Message, "harness execution failed") {
		t.Fatalf("expected execution failure message, got %q", result.Issues[0].Message)
	}
	if got := evidenceValue(result.Evidence, "runtime.stderr"); got == "" {
		t.Fatal("expected runtime.stderr evidence")
	}
}

func TestHarnessRunnerParseFailureBecomesIssue(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			return []byte("Traceback (most recent call last):\nModuleNotFoundError: boom\n"), nil, nil
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: t.TempDir()},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("expected no Go error, got %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to be enabled")
	}
	if !hasIssueCode(result.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected harness parse failure issue, got %+v", result.Issues)
	}
	if !strings.Contains(result.Issues[0].Message, "harness output parse failed") {
		t.Fatalf("expected parse failure message, got %q", result.Issues[0].Message)
	}
	if got := evidenceValue(result.Evidence, "harness.stdout_noise"); got == "" {
		t.Fatal("expected harness.stdout_noise evidence")
	}
}

func TestHarnessRunnerStdoutNoiseIsRecordedAsEvidence(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			stdout := "WARNING: numpy deprecation\n{\"event\":\"summary\",\"status\":\"ok\",\"message\":\"done\"}\n"
			return []byte(stdout), nil, nil
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: t.TempDir()},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if got := evidenceValue(result.Evidence, "harness.stdout_noise"); got != "WARNING: numpy deprecation" {
		t.Fatalf("expected noise evidence, got %q", got)
	}
}

func TestHarnessRunnerTimeout(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	runner := NewHarnessRunner()
	runner.timeout = 20 * time.Millisecond
	runner.scriptPath = writeHarnessStubScript(t)
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			<-ctx.Done()
			return nil, nil, ctx.Err()
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: t.TempDir()},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("expected no Go error after timeout, got %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to be enabled")
	}
	if !hasIssueCode(result.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected harness failure issue, got %+v", result.Issues)
	}
	if !strings.Contains(result.Issues[0].Message, "harness execution failed") {
		t.Fatalf("expected execution failure message, got %q", result.Issues[0].Message)
	}
}

func TestHarnessRunnerSuccessPath(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	root := t.TempDir()
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			if dir != root {
				t.Fatalf("expected run dir %q, got %q", root, dir)
			}
			return []byte("{\"event\":\"runtime_failed\",\"status\":\"failed\",\"message\":\"bad\"}\n"), nil, nil
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: root},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath: "/tmp/profile/bin/python",
			PoetryVersion:   "Poetry 2.1.0",
			PythonVersion:   "Python 3.12.1",
		},
		Runtime: core.RuntimeState{
			PoetryVersion:         "Poetry runtime fallback",
			ResolvedInterpreter:   "/tmp/runtime/bin/python",
			ResolvedPythonVersion: "Python runtime fallback",
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to be enabled")
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected one parsed event, got %d", len(result.Events))
	}
	if !hasIssueCode(result.Issues, core.IssueCodeHarnessValidationFailed) {
		t.Fatalf("expected issue code %q in %+v", core.IssueCodeHarnessValidationFailed, result.Issues)
	}
	if got := evidenceValue(result.Evidence, "runtime.interpreter_path"); got != "/tmp/profile/bin/python" {
		t.Fatalf("expected runtime profile interpreter path, got %q", got)
	}
	if got := evidenceValue(result.Evidence, "runtime.python_version"); got != "Python 3.12.1" {
		t.Fatalf("expected runtime profile python version, got %q", got)
	}
	if got := evidenceValue(result.Evidence, "runtime.poetry_version"); got != "Poetry 2.1.0" {
		t.Fatalf("expected runtime profile poetry version, got %q", got)
	}
}

func TestHarnessRunnerDefaultsToEnabledWhenUnset(t *testing.T) {
	root := t.TempDir()
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.getEnv = func(key string) string {
		return ""
	}
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			return []byte("{\"event\":\"summary\",\"status\":\"ok\",\"message\":\"done\"}\n"), nil, nil
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: root},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to default to enabled when env var is unset")
	}
}

func TestHarnessRunnerRespectsEnablementEnvVar(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "0")
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)

	called := false
	runner.runtimeRunner = &PythonRuntimeRunner{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			called = true
			return nil, nil, nil
		},
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{
		Repository:     core.RepositoryState{Root: t.TempDir()},
		RuntimeProfile: &core.LocalRuntimeProfile{InterpreterPath: "/tmp/venv/bin/python"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Enabled {
		t.Fatal("expected harness to be disabled")
	}
	if called {
		t.Fatal("did not expect harness command to run when disabled")
	}
}

func TestHarnessRunnerResolvesDefaultScriptOutsideRepoScriptsDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}

	repoRoot := t.TempDir()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	runner := NewHarnessRunner()

	scriptPath, err := runner.resolveScriptPath()
	if err != nil {
		t.Fatalf("resolveScriptPath returned error: %v", err)
	}
	if !filepath.IsAbs(scriptPath) {
		t.Fatalf("expected absolute script path, got %q", scriptPath)
	}
	if strings.HasPrefix(scriptPath, filepath.Join(repoRoot, "scripts")+string(os.PathSeparator)) {
		t.Fatalf("expected harness assets outside repo scripts dir, got %q", scriptPath)
	}
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("expected script to exist at %q: %v", scriptPath, err)
	}

	harnessLibRunner := filepath.Join(filepath.Dir(scriptPath), "harness_lib", "runner.py")
	if _, err := os.Stat(harnessLibRunner); err != nil {
		t.Fatalf("expected bundled harness library at %q: %v", harnessLibRunner, err)
	}
}

func writeHarnessStubScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "harness_stub.py")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env python3\nprint('{}')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	return path
}

func evidenceValue(evidence []core.EvidenceItem, name string) string {
	for _, item := range evidence {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
