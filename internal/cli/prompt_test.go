package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptProjectRootSelectionReturnsChosenCandidate(t *testing.T) {
	candidates := []string{"/repo/a", "/repo/b"}
	input := bytes.NewBufferString("2\n")
	output := new(bytes.Buffer)

	selected, err := promptProjectRootSelection(input, output, candidates)
	if err != nil {
		t.Fatalf("promptProjectRootSelection returned error: %v", err)
	}
	if selected != "/repo/b" {
		t.Fatalf("expected selected candidate %q, got %q", "/repo/b", selected)
	}
}

func TestPromptModelCandidateSelectionReturnsChosenCandidate(t *testing.T) {
	candidates := []string{"model/a.h5", "model/b.onnx"}
	input := bytes.NewBufferString("2\n")
	output := new(bytes.Buffer)

	selected, err := promptModelCandidateSelection(input, output, candidates)
	if err != nil {
		t.Fatalf("promptModelCandidateSelection returned error: %v", err)
	}
	if selected != "model/b.onnx" {
		t.Fatalf("expected selected candidate %q, got %q", "model/b.onnx", selected)
	}
}

func TestPromptApprovalParsesYesNo(t *testing.T) {
	approved, err := promptApproval(bytes.NewBufferString("yes\n"), new(bytes.Buffer), "approve", false)
	if err != nil {
		t.Fatalf("promptApproval yes returned error: %v", err)
	}
	if !approved {
		t.Fatal("expected approval for yes")
	}

	rejected, err := promptApproval(bytes.NewBufferString("n\n"), new(bytes.Buffer), "approve", false)
	if err != nil {
		t.Fatalf("promptApproval no returned error: %v", err)
	}
	if rejected {
		t.Fatal("expected rejection for no")
	}
}

func TestPromptYesNoDefaultsToYesWhenConfigured(t *testing.T) {
	approved, err := promptYesNo(bytes.NewBufferString("\n"), new(bytes.Buffer), "Apply and commit these changes? [Y/n]:", true)
	if err != nil {
		t.Fatalf("promptYesNo returned error: %v", err)
	}
	if !approved {
		t.Fatal("expected empty response to use default yes")
	}
}

func TestDetectProjectRootCandidatesFindsSiblingGitRepos(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "repo-a")
	repoB := filepath.Join(root, "repo-b")
	if err := os.MkdirAll(filepath.Join(repoA, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll repoA failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoB, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll repoB failed: %v", err)
	}
	writeFile(t, filepath.Join(repoA, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeFile(t, filepath.Join(repoB, ".git", "HEAD"), "ref: refs/heads/main\n")

	candidates, err := detectProjectRootCandidates(root)
	if err != nil {
		t.Fatalf("detectProjectRootCandidates returned error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %v", len(candidates), candidates)
	}
	expectedA, err := canonicalPath(repoA)
	if err != nil {
		t.Fatalf("canonicalPath repoA failed: %v", err)
	}
	expectedB, err := canonicalPath(repoB)
	if err != nil {
		t.Fatalf("canonicalPath repoB failed: %v", err)
	}
	if candidates[0] != expectedA {
		t.Fatalf("expected first candidate %q, got %q", expectedA, candidates[0])
	}
	if candidates[1] != expectedB {
		t.Fatalf("expected second candidate %q, got %q", expectedB, candidates[1])
	}
}
