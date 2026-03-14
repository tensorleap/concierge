package execute

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPoetryDependencyExecutorInstallsWhenCodeLoaderDeclaredButMissingFromEnv(t *testing.T) {
	t.Parallel()

	var calls []string
	executor := &PoetryDependencyExecutor{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			call := name + " " + strings.Join(args, " ")
			calls = append(calls, call)
			switch call {
			case "poetry install":
				return []byte("Installing dependencies\n"), nil, nil
			case "/repo/.venv/bin/python -c import code_loader":
				return nil, nil, nil
			default:
				t.Fatalf("unexpected command: %q", call)
				return nil, nil, nil
			}
		},
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: "/repo"},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath:            "/repo/.venv/bin/python",
			CodeLoaderReady:            false,
			CodeLoaderDeclaredInProject: true,
		},
	}, core.EnsureStep{ID: core.EnsureStepPythonRuntime})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !result.Applied {
		t.Fatalf("expected applied result, got %+v", result)
	}
	if got := strings.Join(calls, "\n"); strings.Contains(got, "poetry add code-loader") {
		t.Fatalf("did not expect poetry add for declared dependency, got calls:\n%s", got)
	}
	if want := "installed project dependencies in the resolved Poetry environment and verified `code_loader` import"; result.Summary != want {
		t.Fatalf("expected summary %q, got %q", want, result.Summary)
	}
	assertPoetryEvidenceValue(t, result.Evidence, "runtime.command", "poetry install")
	assertPoetryEvidenceValue(t, result.Evidence, "runtime.verification.command", "/repo/.venv/bin/python -c import code_loader")
	assertPoetryEvidenceValue(t, result.Evidence, "runtime.verification.result", "code_loader import ok")
}

func TestPoetryDependencyExecutorAddsCodeLoaderWhenProjectDoesNotDeclareIt(t *testing.T) {
	t.Parallel()

	var calls []string
	executor := &PoetryDependencyExecutor{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			call := name + " " + strings.Join(args, " ")
			calls = append(calls, call)
			switch call {
			case "poetry add code-loader@^1.0.165":
				return []byte("Adding code-loader\n"), nil, nil
			case "/repo/.venv/bin/python -c import code_loader":
				return nil, nil, nil
			default:
				t.Fatalf("unexpected command: %q", call)
				return nil, nil, nil
			}
		},
	}

	result, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: "/repo"},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath:            "/repo/.venv/bin/python",
			CodeLoaderReady:            false,
			CodeLoaderDeclaredInProject: false,
		},
	}, core.EnsureStep{ID: core.EnsureStepPythonRuntime})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !result.Applied {
		t.Fatalf("expected applied result, got %+v", result)
	}
	if want := "added code-loader@^1.0.165 through Poetry and verified `code_loader` import"; result.Summary != want {
		t.Fatalf("expected summary %q, got %q", want, result.Summary)
	}
	assertPoetryEvidenceValue(t, result.Evidence, "runtime.command", "poetry add code-loader@^1.0.165")
	assertPoetryEvidenceValue(t, result.Evidence, "runtime.verification.result", "code_loader import ok")
}

func TestPoetryDependencyExecutorReturnsErrorWhenCodeLoaderVerificationStillFails(t *testing.T) {
	t.Parallel()

	executor := &PoetryDependencyExecutor{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			call := name + " " + strings.Join(args, " ")
			switch call {
			case "poetry install":
				return []byte("Installing dependencies\n"), nil, nil
			case "/repo/.venv/bin/python -c import code_loader":
				return nil, []byte("ModuleNotFoundError: No module named 'code_loader'\n"), assertErr("exit status 1")
			default:
				t.Fatalf("unexpected command: %q", call)
				return nil, nil, nil
			}
		},
	}

	_, err := executor.Execute(context.Background(), core.WorkspaceSnapshot{
		Repository: core.RepositoryState{Root: "/repo"},
		RuntimeProfile: &core.LocalRuntimeProfile{
			InterpreterPath:            "/repo/.venv/bin/python",
			CodeLoaderReady:            false,
			CodeLoaderDeclaredInProject: true,
		},
	}, core.EnsureStep{ID: core.EnsureStepPythonRuntime})
	if err == nil {
		t.Fatal("expected verification error")
	}
	if got := err.Error(); !strings.Contains(got, "code_loader import still failed after runtime repair") {
		t.Fatalf("expected verification failure message, got %v", err)
	}
}

func assertPoetryEvidenceValue(t *testing.T, items []core.EvidenceItem, name, want string) {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			if item.Value != want {
				t.Fatalf("expected evidence %q=%q, got %q", name, want, item.Value)
			}
			return
		}
	}
	t.Fatalf("expected evidence %q=%q, got %+v", name, want, items)
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
