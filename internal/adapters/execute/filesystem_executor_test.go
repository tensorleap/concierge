package execute

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
	"gopkg.in/yaml.v3"
)

func TestExecutorCreatesLeapYAMLWhenMissing(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap.yaml is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap.yaml")); err != nil {
		t.Fatalf("expected leap.yaml to be created: %v", err)
	}
	assertEvidenceValue(t, result.Evidence, "executor.before_checksum", "missing")
}

func TestExecutorCreatesIntegrationScriptTemplate(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationScript)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap_binder.py is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap_binder.py")); err != nil {
		t.Fatalf("expected leap_binder.py to be created: %v", err)
	}
}

func TestExecutorCreatesIntegrationTestTemplate(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when leap_custom_test.py is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap_custom_test.py")); err != nil {
		t.Fatalf("expected leap_custom_test.py to be created: %v", err)
	}
}

func TestExecutorIdempotentOnSecondRun(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	first, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !first.Applied {
		t.Fatal("expected first run to apply changes")
	}

	second, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if second.Applied {
		t.Fatal("expected second run to be idempotent with applied=false")
	}
	before := evidenceValue(second.Evidence, "executor.before_checksum")
	after := evidenceValue(second.Evidence, "executor.after_checksum")
	if before == "" || after == "" {
		t.Fatalf("expected checksum evidence on second run, got %+v", second.Evidence)
	}
	if before != after {
		t.Fatalf("expected matching checksums on idempotent second run, got before=%q after=%q", before, after)
	}
}

func TestExecutorRepairsLeapYAMLIncludeAndExcludeRules(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), "def noop():\n    return None\n")
	writeFile(t, filepath.Join(repoRoot, "leap_custom_test.py"), "def test_noop():\n    return None\n")
	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), strings.Join([]string{
		"entryFile: leap_binder.py",
		"include:",
		"  - leap_binder.py",
		"exclude:",
		"  - leap.yaml",
		"  - leap_custom_test.py",
		"  - .git/**",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected leap.yaml repair to apply changes")
	}
	if !strings.Contains(result.Summary, "updated leap.yaml upload rules") {
		t.Fatalf("expected upload-rules summary, got %q", result.Summary)
	}

	contract := readLeapYAMLContract(t, filepath.Join(repoRoot, "leap.yaml"))
	assertContainsAll(t, contract.Include, []string{"leap.yaml", "leap_binder.py", "leap_custom_test.py"})
	assertContainsNone(t, contract.Exclude, []string{"leap.yaml", "leap_custom_test.py"})
	if !contains(contract.Exclude, ".git/**") {
		t.Fatalf("expected non-blocking exclude pattern to remain, got %v", contract.Exclude)
	}
}

func TestExecutorRepairsLeapYAMLMissingEntryFile(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), "def noop():\n    return None\n")
	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), strings.Join([]string{
		"include:",
		"  - leap.yaml",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected leap.yaml entryFile repair to apply changes")
	}
	if !strings.Contains(result.Summary, "entryFile") {
		t.Fatalf("expected entryFile-focused summary, got %q", result.Summary)
	}

	contract := readLeapYAMLContract(t, filepath.Join(repoRoot, "leap.yaml"))
	if contract.EntryFile != "leap_binder.py" {
		t.Fatalf("expected repaired entryFile %q, got %q", "leap_binder.py", contract.EntryFile)
	}
}

func TestExecutorCreatesEntryFileWhenLeapYAMLEntryFileMissingOnDisk(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), strings.Join([]string{
		"entryFile: leap_binder.py",
		"include:",
		"  - leap.yaml",
		"  - leap_binder.py",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected leap.yaml step to scaffold missing entryFile target")
	}
	if !strings.Contains(result.Summary, "created leap_binder.py") {
		t.Fatalf("expected summary to mention entry file scaffold, got %q", result.Summary)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "leap_binder.py")); err != nil {
		t.Fatalf("expected leap_binder.py to be created: %v", err)
	}
	assertEvidenceValue(t, result.Evidence, "executor.entry_file", "leap_binder.py")
	assertEvidenceValue(t, result.Evidence, "executor.entry_file.before_checksum", "missing")
	entryAfter := evidenceValue(result.Evidence, "executor.entry_file.after_checksum")
	if entryAfter == "" || entryAfter == "missing" {
		t.Fatalf("expected entry file after checksum evidence, got %q", entryAfter)
	}
}

func TestExecutorDoesNotModifyCompliantLeapYAML(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, "leap_binder.py"), "def noop():\n    return None\n")
	writeFile(t, filepath.Join(repoRoot, "leap_custom_test.py"), "def test_noop():\n    return None\n")
	initial := strings.Join([]string{
		"entryFile: leap_binder.py",
		"include:",
		"  - leap.yaml",
		"  - leap_binder.py",
		"  - leap_custom_test.py",
		"exclude:",
		"  - .git/**",
		"",
	}, "\n")
	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), initial)

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Applied {
		t.Fatal("expected compliant leap.yaml to remain unchanged")
	}
	if !strings.Contains(result.Summary, "already satisfies") {
		t.Fatalf("expected idempotent summary, got %q", result.Summary)
	}

	afterRaw, err := os.ReadFile(filepath.Join(repoRoot, "leap.yaml"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(afterRaw) != initial {
		t.Fatalf("expected leap.yaml to remain unchanged, got:\n%s", string(afterRaw))
	}
}

func snapshotForRepo(root string) core.WorkspaceSnapshot {
	return core.WorkspaceSnapshot{Repository: core.RepositoryState{Root: root}}
}

func assertEvidenceValue(t *testing.T, evidence []core.EvidenceItem, name string, expected string) {
	t.Helper()
	value := evidenceValue(evidence, name)
	if value != expected {
		t.Fatalf("expected evidence %q=%q, got %q", name, expected, value)
	}
}

func evidenceValue(evidence []core.EvidenceItem, name string) string {
	for _, item := range evidence {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}

type leapYAMLContract struct {
	EntryFile string   `yaml:"entryFile"`
	Include   []string `yaml:"include"`
	Exclude   []string `yaml:"exclude"`
}

func readLeapYAMLContract(t *testing.T, path string) leapYAMLContract {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var contract leapYAMLContract
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("Unmarshal failed: %v\ncontent:\n%s", err, string(raw))
	}
	return contract
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func assertContainsAll(t *testing.T, values []string, expected []string) {
	t.Helper()
	for _, candidate := range expected {
		if !contains(values, candidate) {
			t.Fatalf("expected %q in %v", candidate, values)
		}
	}
}

func assertContainsNone(t *testing.T, values []string, denied []string) {
	t.Helper()
	for _, candidate := range denied {
		if contains(values, candidate) {
			t.Fatalf("did not expect %q in %v", candidate, values)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
