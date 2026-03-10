package inspect

import (
	"context"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPoetryRuntimeResolverRechecksReadinessWhenPreviousProfileWasStale(t *testing.T) {
	t.Parallel()

	callCount := 0
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name
			callCount++

			switch strings.Join(args, " ") {
			case "env info --executable":
				return []byte("/repo/.venv/bin/python\n"), nil, nil
			case "run python --version":
				return []byte("Python 3.11.8\n"), nil, nil
			case "check":
				return []byte("All set!\n"), nil, nil
			case "run python -c import code_loader":
				return nil, nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", strings.Join(args, " "))
				return nil, nil, nil
			}
		},
	}

	snapshot := core.WorkspaceSnapshot{
		FileHashes: map[string]string{
			"pyproject.toml": "pyproject-hash",
			"poetry.lock":    "poetry-lock-hash",
		},
		Runtime: core.RuntimeState{
			SupportedProject: true,
			PoetryFound:      true,
			PoetryExecutable: "poetry",
			PoetryVersion:    "Poetry 2.0.0",
		},
	}
	previous := &core.LocalRuntimeProfile{
		Kind:              "poetry",
		PoetryExecutable:  "poetry",
		PoetryVersion:     "Poetry 2.0.0",
		InterpreterPath:   "/repo/.venv/bin/python",
		PythonVersion:     "Python 3.11.8",
		DependenciesReady: false,
		CodeLoaderReady:   false,
		Fingerprint: core.RuntimeProfileFingerprint{
			ProjectRoot:     "/repo",
			PyProjectHash:   "pyproject-hash",
			PoetryLockHash:  "poetry-lock-hash",
			InterpreterPath: "/repo/.venv/bin/python",
			PythonVersion:   "Python 3.11.8",
		},
	}

	resolution, err := resolver.Resolve(context.Background(), "/repo", snapshot, previous)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Profile == nil {
		t.Fatal("expected resolved runtime profile")
	}
	if !resolution.Profile.DependenciesReady {
		t.Fatalf("expected DependenciesReady to be re-probed as true, got false")
	}
	if !resolution.Profile.CodeLoaderReady {
		t.Fatalf("expected CodeLoaderReady to be re-probed as true, got false")
	}
	if callCount == 0 {
		t.Fatal("expected resolver to call Poetry instead of reusing the stale profile")
	}
}

func TestPoetryRuntimeResolverFlagsInterpreterDriftWithoutLockfileChange(t *testing.T) {
	t.Parallel()

	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name

			switch strings.Join(args, " ") {
			case "env info --executable":
				return []byte("/repo/.venv-alt/bin/python\n"), nil, nil
			case "run python --version":
				return []byte("Python 3.11.8\n"), nil, nil
			case "check":
				return []byte("All set!\n"), nil, nil
			case "run python -c import code_loader":
				return nil, nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", strings.Join(args, " "))
				return nil, nil, nil
			}
		},
	}

	snapshot := core.WorkspaceSnapshot{
		FileHashes: map[string]string{
			"pyproject.toml": "pyproject-hash",
			"poetry.lock":    "poetry-lock-hash",
		},
		Runtime: core.RuntimeState{
			SupportedProject: true,
			PoetryFound:      true,
			PoetryExecutable: "poetry",
			PoetryVersion:    "Poetry 2.0.0",
		},
	}
	previous := &core.LocalRuntimeProfile{
		Kind:             "poetry",
		PoetryExecutable: "poetry",
		PoetryVersion:    "Poetry 2.0.0",
		InterpreterPath:  "/repo/.venv/bin/python",
		PythonVersion:    "Python 3.11.8",
		Fingerprint: core.RuntimeProfileFingerprint{
			ProjectRoot:     "/repo",
			PyProjectHash:   "pyproject-hash",
			PoetryLockHash:  "poetry-lock-hash",
			InterpreterPath: "/repo/.venv/bin/python",
			PythonVersion:   "Python 3.11.8",
		},
	}

	resolution, err := resolver.Resolve(context.Background(), "/repo", snapshot, previous)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Profile == nil {
		t.Fatal("expected resolved runtime profile")
	}
	if resolution.Profile.InterpreterPath != "/repo/.venv-alt/bin/python" {
		t.Fatalf("expected fresh interpreter path, got %q", resolution.Profile.InterpreterPath)
	}
	if len(resolution.SuspiciousReasons) != 1 {
		t.Fatalf("expected one suspicious reason, got %d: %v", len(resolution.SuspiciousReasons), resolution.SuspiciousReasons)
	}
	if got := resolution.SuspiciousReasons[0]; !strings.Contains(got, "different interpreter") {
		t.Fatalf("expected interpreter-drift warning, got %q", got)
	}
}
