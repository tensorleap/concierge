package fixtures

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/validate"
	"github.com/tensorleap/concierge/internal/state"
)

func TestFixturePostVariantsPassConciergeRunWithPreparedRuntime(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")
	mockFixtureCLIs(t)

	repoRoot := repoRootFromRuntime(t)
	binaryPath := filepath.Join(t.TempDir(), "concierge")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/concierge")
	buildCmd.Dir = repoRoot
	buildCmd.Env = os.Environ()
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, buildOutput)
	}

	for _, fixture := range loadFixtures(t) {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			_, postRoot := resolveFixtureRoots(t, fixture.ID)
			postStatus := inspectStatus(t, postRoot)
			if reason := stalePreparedPostFixtureReason(postStatus.Issues); reason != "" {
				t.Skipf("skipping fixture %q until prepared post fixtures are regenerated for GUIDE1: %s", fixture.ID, reason)
			}

			hasModel, err := repoHasSupportedModelArtifact(postRoot)
			if err != nil {
				t.Fatalf("repoHasSupportedModelArtifact failed: %v", err)
			}
			if !hasModel {
				t.Skipf("skipping fixture %q because no local supported model artifact is available", fixture.ID)
			}

			runRoot := cloneFixtureRepoForTest(t, postRoot)
			seedSelectedModelSourceIfNeeded(t, runRoot)
			cmd := exec.Command(
				binaryPath,
				"run",
				"--project-root="+runRoot,
				"--yes",
				"--max-iterations=1",
				"--no-color",
			)
			cmd.Dir = repoRoot
			cmd.Env = os.Environ()

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("concierge run failed for fixture %q: %v\n%s", fixture.ID, err, output)
			}
			if strings.Contains(string(output), "Missing integration step: Poetry environment should be available and have the required packages") {
				t.Fatalf("fixture %q still failed runtime gating:\n%s", fixture.ID, output)
			}
		})
	}
}

func mockFixtureCLIs(t *testing.T) string {
	t.Helper()

	binDir := t.TempDir()
	leapPath := filepath.Join(binDir, "leap")
	poetryPath := filepath.Join(binDir, "poetry")
	claudePath := filepath.Join(binDir, "claude")
	fixturePythonPath := filepath.Join(binDir, "fixture-python")

	leapScript := `#!/usr/bin/env bash
set -euo pipefail

cmd="${1:-}"
case "$cmd" in
  --version)
    echo "leap v0.2.0"
    ;;
  auth)
    if [[ "${2:-}" != "whoami" ]]; then
      echo "unsupported auth subcommand" >&2
      exit 1
    fi
    echo "fixtures@example.com"
    ;;
  server)
    if [[ "${2:-}" != "info" ]]; then
      echo "unsupported server subcommand" >&2
      exit 1
    fi
    cat <<'EOF'
Installation information:
datasetvolumes: []
EOF
    ;;
  *)
    echo "unsupported leap command" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(leapPath, []byte(leapScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock leap CLI: %v", err)
	}

	fixturePythonScript := `#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "--version" ]]; then
  echo "Python 3.10.13"
  exit 0
fi

if [[ "${1:-}" == "-c" ]]; then
  code="${2:-}"
  if [[ "$code" == "import code_loader" ]]; then
    exit 0
  fi
  if [[ "$code" == "import sys; print(sys.executable)" ]]; then
    echo "$0"
    exit 0
  fi
  if [[ "$code" == *"supportsGuideLocalStatusTable"* ]]; then
    echo '{"probeSucceeded":true,"version":"1.0.165","supportsGuideLocalStatusTable":true,"supportsCheckDataset":true}'
    exit 0
  fi
  if [[ "$code" == *"import onnx"* || "$code" == *"from tensorflow import keras"* ]]; then
    echo '{"inputs":[]}'
    exit 0
  fi
  echo "{}"
  exit 0
fi

python3 "$@"
`
	if err := os.WriteFile(fixturePythonPath, []byte(fixturePythonScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock fixture python: %v", err)
	}

	poetryScript := `#!/usr/bin/env bash
set -euo pipefail

fixture_python="` + fixturePythonPath + `"

cmd="${1:-}"
case "$cmd" in
  --version)
    echo "Poetry 2.0.0"
    ;;
  env)
    if [[ "${2:-}" != "info" || "${3:-}" != "--executable" ]]; then
      echo "unsupported poetry env command" >&2
      exit 1
    fi
    echo "$fixture_python"
    ;;
  check)
    echo "All set!"
    ;;
  install)
    echo "Installing dependencies"
    ;;
  add)
    echo "Adding dependency ${2:-}"
    ;;
  run)
    if [[ "${2:-}" != "python" ]]; then
      echo "unsupported poetry run command" >&2
      exit 1
    fi
    "$fixture_python" "${@:3}"
    ;;
  *)
    echo "unsupported poetry command" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(poetryPath, []byte(poetryScript), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock poetry CLI: %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("#!/usr/bin/env bash\necho 'unexpected claude invocation in fixture acceptance test' >&2\nexit 97\n"), 0o755); err != nil {
		t.Fatalf("WriteFile failed for mock claude CLI: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}

func seedSelectedModelSourceIfNeeded(t *testing.T, projectRoot string) {
	t.Helper()

	candidates, err := supportedModelArtifacts(projectRoot)
	if err != nil {
		t.Fatalf("supportedModelArtifacts failed: %v", err)
	}
	if len(candidates) <= 1 {
		return
	}

	selected := candidates[0]
	runState := state.DefaultRunState(projectRoot)
	runState.SelectedModelPath = selected
	runState.ModelAcquisitionClarification = &state.ModelAcquisitionClarification{
		SelectedVerifiedModelPath: selected,
	}
	if err := state.SaveState(projectRoot, runState); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
}

func supportedModelArtifacts(repoRoot string) ([]string, error) {
	candidates := make([]string, 0, 4)
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".onnx", ".h5", ".keras":
		default:
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		candidates = append(candidates, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(candidates)
	return candidates, nil
}
