package validate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

func TestHarnessRunnerTimeout(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	runner := NewHarnessRunner()
	runner.timeout = 20 * time.Millisecond
	runner.scriptPath = writeHarnessStubScript(t)
	runner.lookPath = func(file string) (string, error) {
		return "/usr/bin/python3", nil
	}
	runner.runCommand = func(ctx context.Context, dir, command string, args ...string) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	_, err := runner.Run(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: t.TempDir()}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestHarnessRunnerSuccessPath(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "1")
	root := t.TempDir()
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.lookPath = func(file string) (string, error) {
		return "/usr/bin/python3", nil
	}
	runner.runCommand = func(ctx context.Context, dir, command string, args ...string) ([]byte, []byte, error) {
		if dir != root {
			t.Fatalf("expected run dir %q, got %q", root, dir)
		}
		return []byte("{\"event\":\"validation\",\"status\":\"failed\",\"message\":\"bad\"}\n"), nil, nil
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: root}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.Enabled {
		t.Fatal("expected harness to be enabled")
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected one parsed event, got %d", len(result.Events))
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected one issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Code != core.IssueCodeHarnessValidationFailed {
		t.Fatalf("expected issue code %q, got %q", core.IssueCodeHarnessValidationFailed, result.Issues[0].Code)
	}
}

func TestHarnessRunnerRespectsEnablementEnvVar(t *testing.T) {
	t.Setenv(HarnessEnableEnvVar, "0")
	runner := NewHarnessRunner()
	runner.scriptPath = writeHarnessStubScript(t)
	runner.lookPath = func(file string) (string, error) {
		return "/usr/bin/python3", nil
	}

	called := false
	runner.runCommand = func(ctx context.Context, dir, command string, args ...string) ([]byte, []byte, error) {
		called = true
		return nil, nil, nil
	}

	result, err := runner.Run(context.Background(), core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: t.TempDir()}})
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

func writeHarnessStubScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "harness_stub.py")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env python3\nprint('{}')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	return path
}
