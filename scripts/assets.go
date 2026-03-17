package scripts

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

//go:embed harness_runtime.py harness_lib/*.py
var harnessAssetsFS embed.FS

var (
	harnessAssetsOnce       sync.Once
	harnessAssetsScriptPath string
	harnessAssetsErr        error
)

// HarnessRuntimePath materializes the bundled runtime harness assets once and
// returns the absolute path to the entrypoint script.
func HarnessRuntimePath() (string, error) {
	harnessAssetsOnce.Do(func() {
		harnessAssetsScriptPath, harnessAssetsErr = materializeHarnessAssets()
	})
	return harnessAssetsScriptPath, harnessAssetsErr
}

func materializeHarnessAssets() (string, error) {
	root, err := os.MkdirTemp("", "concierge-harness-*")
	if err != nil {
		return "", err
	}

	if err := writeHarnessAsset(root, "harness_runtime.py", 0o755); err != nil {
		return "", err
	}
	if err := writeHarnessAsset(root, "harness_lib/events.py", 0o644); err != nil {
		return "", err
	}
	if err := writeHarnessAsset(root, "harness_lib/runner.py", 0o644); err != nil {
		return "", err
	}

	return filepath.Join(root, "harness_runtime.py"), nil
}

func writeHarnessAsset(root, assetPath string, mode fs.FileMode) error {
	contents, err := harnessAssetsFS.ReadFile(assetPath)
	if err != nil {
		return err
	}

	destination := filepath.Join(root, filepath.FromSlash(assetPath))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, contents, mode); err != nil {
		return err
	}
	return nil
}
