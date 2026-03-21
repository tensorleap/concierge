package execute

import (
	"context"
	"fmt"
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
		t.Fatal("expected applied=true when leap_integration.py is missing")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile)); err != nil {
		t.Fatalf("expected %s to be created: %v", core.CanonicalIntegrationEntryFile, err)
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
		t.Fatal("expected applied=true when integration_test scaffold is missing")
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile))
	if err != nil {
		t.Fatalf("expected %s to be created: %v", core.CanonicalIntegrationEntryFile, err)
	}
	if !strings.Contains(string(raw), "@tensorleap_integration_test") {
		t.Fatalf("expected integration-test scaffold in %s, got:\n%s", core.CanonicalIntegrationEntryFile, string(raw))
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

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), "def noop():\n    return None\n")
	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), strings.Join([]string{
		"entryFile: old_entry.py",
		"include:",
		"  - old_entry.py",
		"exclude:",
		"  - leap.yaml",
		"  - leap_integration.py",
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
	if !strings.Contains(result.Summary, "updated leap.yaml entryFile and upload rules") {
		t.Fatalf("expected entryFile/upload-rules summary, got %q", result.Summary)
	}

	contract := readLeapYAMLContract(t, filepath.Join(repoRoot, "leap.yaml"))
	assertContainsAll(t, contract.Include, []string{"leap.yaml", "leap_integration.py"})
	assertContainsNone(t, contract.Exclude, []string{"leap.yaml", "leap_integration.py"})
	if !contains(contract.Exclude, ".git/**") {
		t.Fatalf("expected non-blocking exclude pattern to remain, got %v", contract.Exclude)
	}
}

func TestExecutorRepairsLeapYAMLMissingEntryFile(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), "def noop():\n    return None\n")
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
	if contract.EntryFile != core.CanonicalIntegrationEntryFile {
		t.Fatalf("expected repaired entryFile %q, got %q", core.CanonicalIntegrationEntryFile, contract.EntryFile)
	}
}

func TestExecutorCreatesEntryFileWhenLeapYAMLEntryFileMissingOnDisk(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepLeapYAML)

	writeFile(t, filepath.Join(repoRoot, "leap.yaml"), strings.Join([]string{
		fmt.Sprintf("entryFile: %s", core.CanonicalIntegrationEntryFile),
		"include:",
		"  - leap.yaml",
		"  - leap_integration.py",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected leap.yaml step to scaffold missing entryFile target")
	}
	if !strings.Contains(result.Summary, "created leap_integration.py") {
		t.Fatalf("expected summary to mention entry file scaffold, got %q", result.Summary)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile)); err != nil {
		t.Fatalf("expected %s to be created: %v", core.CanonicalIntegrationEntryFile, err)
	}
	assertEvidenceValue(t, result.Evidence, "executor.entry_file", core.CanonicalIntegrationEntryFile)
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

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), "def noop():\n    return None\n")
	initial := strings.Join([]string{
		fmt.Sprintf("entryFile: %s", core.CanonicalIntegrationEntryFile),
		"include:",
		"  - leap.yaml",
		"  - leap_integration.py",
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

func TestExecutorAddsMainBlockWhenPreprocessExists(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), strings.Join([]string{
		`"""Baseline Tensorleap integration entrypoint."""`,
		"",
		"from code_loader.inner_leap_binder.leapbinder_decorators import tensorleap_preprocess",
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func():",
		"    return []",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when __main__ block is missing")
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "@tensorleap_integration_test") {
		t.Fatal("expected integration-test scaffold to be present")
	}
	if !strings.Contains(content, `if __name__ == "__main__":`) {
		t.Fatal("expected __main__ block to be present")
	}
	if !strings.Contains(content, "preprocess_func()") {
		t.Fatal("expected __main__ block to call preprocess_func()")
	}
	if !strings.Contains(content, "integration_test(sample_id, subset)") {
		t.Fatal("expected __main__ block to call integration_test()")
	}
	if !strings.Contains(content, "subset.sample_ids[:5]") {
		t.Fatal("expected __main__ block to iterate subset.sample_ids")
	}
	if !strings.Contains(result.Summary, "@tensorleap_integration_test scaffold") {
		t.Fatalf("expected summary to mention scaffold, got %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "__main__ entry-point") {
		t.Fatalf("expected summary to mention __main__, got %q", result.Summary)
	}
}

func TestExecutorMainBlockIdempotent(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), strings.Join([]string{
		`"""Baseline Tensorleap integration entrypoint."""`,
		"",
		"@tensorleap_preprocess()",
		"def preprocess_func():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess):",
		"    return None",
		"",
		`if __name__ == "__main__":`,
		"    responses = preprocess_func()",
		"    for subset in responses:",
		"        for sample_id in subset.sample_ids[:5]:",
		"            integration_test(sample_id, subset)",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Applied {
		t.Fatal("expected applied=false when __main__ block already exists")
	}
}

func TestExecutorSkipsMainBlockWithoutPreprocess(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), strings.Join([]string{
		`"""Baseline Tensorleap integration entrypoint."""`,
		"",
		"@tensorleap_integration_test()",
		"def integration_test(sample_id, preprocess):",
		"    return None",
		"",
	}, "\n"))

	_, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(raw), "if __name__") {
		t.Fatal("expected no __main__ block when preprocess function is missing")
	}
}

func TestExecutorMainBlockUsesCorrectFunctionNames(t *testing.T) {
	executor := NewFilesystemExecutor()
	repoRoot := t.TempDir()
	step, _ := core.EnsureStepByID(core.EnsureStepIntegrationTestContract)

	writeFile(t, filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile), strings.Join([]string{
		`"""Custom integration."""`,
		"",
		"@tensorleap_preprocess()",
		"def my_custom_preprocess():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def my_custom_test(sample_id, preprocess):",
		"    return None",
		"",
	}, "\n"))

	result, err := executor.Execute(context.Background(), snapshotForRepo(repoRoot), step)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Applied {
		t.Fatal("expected applied=true when __main__ block is missing")
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, core.CanonicalIntegrationEntryFile))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "my_custom_preprocess()") {
		t.Fatal("expected __main__ block to use discovered preprocess function name")
	}
	if !strings.Contains(content, "my_custom_test(sample_id, subset)") {
		t.Fatal("expected __main__ block to use discovered integration test function name")
	}
}

func TestFindDecoratedFunctionName(t *testing.T) {
	source := strings.Join([]string{
		"@tensorleap_preprocess()",
		"def preprocess_func():",
		"    return []",
		"",
		"@tensorleap_integration_test()",
		"def integration_test(s, p):",
		"    return None",
	}, "\n")

	if name := findDecoratedFunctionName(source, "tensorleap_preprocess"); name != "preprocess_func" {
		t.Fatalf("expected preprocess_func, got %q", name)
	}
	if name := findDecoratedFunctionName(source, "tensorleap_integration_test"); name != "integration_test" {
		t.Fatalf("expected integration_test, got %q", name)
	}
	if name := findDecoratedFunctionName(source, "tensorleap_gt_encoder"); name != "" {
		t.Fatalf("expected empty for missing decorator, got %q", name)
	}
}

func TestFindDecoratedFunctionNameWithModulePrefix(t *testing.T) {
	source := strings.Join([]string{
		"@leapbinder_decorators.tensorleap_preprocess()",
		"def my_preprocess():",
		"    return []",
	}, "\n")

	if name := findDecoratedFunctionName(source, "tensorleap_preprocess"); name != "my_preprocess" {
		t.Fatalf("expected my_preprocess, got %q", name)
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
