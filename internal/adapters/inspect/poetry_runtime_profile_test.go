package inspect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestPoetryRuntimeResolverRechecksReadinessWhenPreviousProfileWasStale(t *testing.T) {
	t.Parallel()

	repoRoot := preparePoetryRuntimeRepo(t, true)
	callCount := 0
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name
			callCount++

			joined := strings.Join(args, " ")
			switch {
			case joined == "env info --executable":
				return []byte("/repo/.venv/bin/python\n"), nil, nil
			case joined == "--version":
				return []byte("Python 3.11.8\n"), nil, nil
			case joined == "check":
				return []byte("All set!\n"), nil, nil
			case joined == "-c import code_loader":
				return nil, nil, nil
			case strings.HasPrefix(joined, "-c ") && strings.Contains(joined, "supportsGuideLocalStatusTable"):
				return []byte(`{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}` + "\n"), nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", joined)
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
			ProjectRoot:     repoRoot,
			PyProjectHash:   "pyproject-hash",
			PoetryLockHash:  "poetry-lock-hash",
			InterpreterPath: "/repo/.venv/bin/python",
			PythonVersion:   "Python 3.11.8",
		},
	}

	resolution, err := resolver.Resolve(context.Background(), repoRoot, snapshot, previous)
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
	if got := resolution.Profile.CodeLoader.Version; got != "1.0.165" {
		t.Fatalf("expected code-loader version %q, got %q", "1.0.165", got)
	}
	if !resolution.Profile.CodeLoader.SupportsGuideLocalStatusTable {
		t.Fatalf("expected guide-local status table support, got %+v", resolution.Profile.CodeLoader)
	}
	if !resolution.Profile.CodeLoaderDeclaredInProject {
		t.Fatalf("expected resolver to record declared code-loader dependency, got %+v", resolution.Profile)
	}
	if callCount == 0 {
		t.Fatal("expected resolver to call Poetry instead of reusing the stale profile")
	}
}

func TestPoetryRuntimeResolverFlagsInterpreterDriftWithoutLockfileChange(t *testing.T) {
	t.Parallel()

	repoRoot := preparePoetryRuntimeRepo(t, true)
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name

			joined := strings.Join(args, " ")
			switch {
			case joined == "env info --executable":
				return []byte("/repo/.venv-alt/bin/python\n"), nil, nil
			case joined == "--version":
				return []byte("Python 3.11.8\n"), nil, nil
			case joined == "check":
				return []byte("All set!\n"), nil, nil
			case joined == "-c import code_loader":
				return nil, nil, nil
			case strings.HasPrefix(joined, "-c ") && strings.Contains(joined, "supportsGuideLocalStatusTable"):
				return []byte(`{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}` + "\n"), nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", joined)
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
			ProjectRoot:     repoRoot,
			PyProjectHash:   "pyproject-hash",
			PoetryLockHash:  "poetry-lock-hash",
			InterpreterPath: "/repo/.venv/bin/python",
			PythonVersion:   "Python 3.11.8",
		},
	}

	resolution, err := resolver.Resolve(context.Background(), repoRoot, snapshot, previous)
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

func TestPoetryRuntimeResolverCapturesLegacyCodeLoaderCapabilities(t *testing.T) {
	t.Parallel()

	repoRoot := preparePoetryRuntimeRepo(t, true)
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name

			joined := strings.Join(args, " ")
			switch {
			case joined == "env info --executable":
				return []byte("/repo/.venv/bin/python\n"), nil, nil
			case joined == "--version":
				return []byte("Python 3.10.16\n"), nil, nil
			case joined == "check":
				return []byte("All set!\n"), nil, nil
			case joined == "-c import code_loader":
				return nil, nil, nil
			case strings.HasPrefix(joined, "-c ") && strings.Contains(joined, "supportsGuideLocalStatusTable"):
				return []byte(`{"probeSucceeded":true,"version":"1.0.138","supportsGuideLocalStatusTable":false,"supportsCheckDataset":true}` + "\n"), nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", joined)
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

	resolution, err := resolver.Resolve(context.Background(), repoRoot, snapshot, nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Profile == nil {
		t.Fatal("expected resolved runtime profile")
	}
	if got := resolution.Profile.CodeLoader.Version; got != "1.0.138" {
		t.Fatalf("expected code-loader version %q, got %q", "1.0.138", got)
	}
	if resolution.Profile.CodeLoader.SupportsGuideLocalStatusTable {
		t.Fatalf("did not expect legacy code-loader to report guide-local status table support: %+v", resolution.Profile.CodeLoader)
	}
	if !resolution.Profile.CodeLoader.SupportsCheckDataset {
		t.Fatalf("expected check_dataset support, got %+v", resolution.Profile.CodeLoader)
	}
}

func TestPoetryRuntimeResolverFallsBackWhenPoetryEnvInfoExecutableFails(t *testing.T) {
	t.Parallel()

	repoRoot := preparePoetryRuntimeRepo(t, true)
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name

			joined := strings.Join(args, " ")
			switch {
			case joined == "env info --executable":
				return nil, []byte("NA\n"), errors.New("exit status 1")
			case joined == "run python -c import sys; print(sys.executable)":
				return []byte("/repo/.venv/bin/python\n"), nil, nil
			case joined == "--version":
				return []byte("Python 3.10.19\n"), nil, nil
			case joined == "check":
				return []byte("All set!\n"), nil, nil
			case joined == "-c import code_loader":
				return nil, nil, nil
			case strings.HasPrefix(joined, "-c ") && strings.Contains(joined, "supportsGuideLocalStatusTable"):
				return []byte(`{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}` + "\n"), nil, nil
			default:
				t.Fatalf("unexpected poetry command: %q", joined)
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

	resolution, err := resolver.Resolve(context.Background(), repoRoot, snapshot, nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Profile == nil {
		t.Fatal("expected resolved runtime profile")
	}
	if resolution.Profile.InterpreterPath != "/repo/.venv/bin/python" {
		t.Fatalf("expected fallback interpreter path, got %q", resolution.Profile.InterpreterPath)
	}
	if resolution.Profile.PythonVersion != "Python 3.10.19" {
		t.Fatalf("expected python version %q, got %q", "Python 3.10.19", resolution.Profile.PythonVersion)
	}
}

func TestPoetryRuntimeResolverDetectsWhenProjectDoesNotDeclareCodeLoader(t *testing.T) {
	t.Parallel()

	repoRoot := preparePoetryRuntimeRepo(t, false)
	resolver := &PoetryRuntimeResolver{
		runCommand: func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			_ = ctx
			_ = dir
			_ = name

			joined := strings.Join(args, " ")
			switch {
			case joined == "env info --executable":
				return []byte("/repo/.venv/bin/python\n"), nil, nil
			case joined == "--version":
				return []byte("Python 3.11.8\n"), nil, nil
			case joined == "check":
				return []byte("All set!\n"), nil, nil
			case joined == "-c import code_loader":
				return nil, []byte("ModuleNotFoundError\n"), errors.New("exit status 1")
			default:
				t.Fatalf("unexpected poetry command: %q", joined)
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

	resolution, err := resolver.Resolve(context.Background(), repoRoot, snapshot, nil)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolution.Profile == nil {
		t.Fatal("expected resolved runtime profile")
	}
	if resolution.Profile.CodeLoaderDeclaredInProject {
		t.Fatalf("did not expect code-loader to be declared in project: %+v", resolution.Profile)
	}
}

func preparePoetryRuntimeRepo(t *testing.T, declared bool) string {
	t.Helper()

	repoRoot := t.TempDir()
	lines := []string{
		"[tool.poetry]",
		`name = "demo"`,
		`version = "0.1.0"`,
		`description = ""`,
		`authors = ["Concierge <qa@example.com>"]`,
		"",
		"[tool.poetry.dependencies]",
		`python = "^3.10"`,
	}
	if declared {
		lines = append(lines, `code-loader = "^1.0.165"`)
	}
	lines = append(lines, "")

	if err := os.WriteFile(filepath.Join(repoRoot, "pyproject.toml"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	return repoRoot
}
