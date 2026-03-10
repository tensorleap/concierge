package fixtures

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/validate"
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
			hasModel, err := repoHasSupportedModelArtifact(postRoot)
			if err != nil {
				t.Fatalf("repoHasSupportedModelArtifact failed: %v", err)
			}
			if !hasModel {
				t.Skipf("skipping fixture %q because no local supported model artifact is available", fixture.ID)
			}

			runRoot := cloneFixtureRepoForTest(t, postRoot)
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

	poetryScript := `#!/usr/bin/env bash
set -euo pipefail

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
    echo "/tmp/concierge-fixtures/.venv/bin/python"
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
    if [[ "${3:-}" == "--version" ]]; then
      echo "Python 3.10.13"
      exit 0
    fi
    if [[ "${3:-}" == "-c" ]]; then
      code="${4:-}"
      if [[ "$code" == "import code_loader" ]]; then
        exit 0
      fi
      if [[ "$code" == *"import onnx"* || "$code" == *"from tensorflow import keras"* ]]; then
        echo '{"inputs":[]}'
        exit 0
      fi
      echo "{}"
      exit 0
    fi
    python3 "${@:3}"
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

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return binDir
}
