package inspect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type frameworkLeadCase struct {
	Name            string            `json:"name"`
	ExpectFramework string            `json:"expect_framework"`
	Files           map[string]string `json:"files"`
}

func TestFrameworkLeadExtractorRanksTrainingPathFilesFirst(t *testing.T) {
	repoRoot := t.TempDir()
	writeFrameworkLeadFile(t, repoRoot, "train.py", strings.Join([]string{
		"import torch",
		"from torch.utils.data import DataLoader",
		"def train(model, ds):",
		"    loader = DataLoader(ds)",
		"    for images, labels in loader:",
		"        model(images)",
		"        loss(model(images), labels)",
		"",
	}, "\n"))
	writeFrameworkLeadFile(t, repoRoot, "utils.py", "def helper(x):\n    return x\n")

	extractor := newFrameworkLeadExtractor()
	leadPack, _, err := extractor.Extract(repoRoot)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(leadPack.Files) == 0 {
		t.Fatalf("expected non-empty lead files")
	}
	if leadPack.Files[0].Path != "train.py" {
		t.Fatalf("expected top lead file %q, got %q", "train.py", leadPack.Files[0].Path)
	}
}

func TestFrameworkLeadExtractorDetectsPyTorchTensorFlowMixedAndUnknown(t *testing.T) {
	cases := loadFrameworkLeadCases(t)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			repoRoot := t.TempDir()
			for relativePath, contents := range tc.Files {
				writeFrameworkLeadFile(t, repoRoot, relativePath, contents)
			}

			extractor := newFrameworkLeadExtractor()
			leadPack, _, err := extractor.Extract(repoRoot)
			if err != nil {
				t.Fatalf("Extract returned error: %v", err)
			}
			if leadPack.FrameworkDetection.Candidate != tc.ExpectFramework {
				t.Fatalf("expected framework %q, got %q", tc.ExpectFramework, leadPack.FrameworkDetection.Candidate)
			}
		})
	}
}

func TestFrameworkLeadExtractorProducesStableOrderingForEqualScores(t *testing.T) {
	repoRoot := t.TempDir()
	sharedContent := strings.Join([]string{
		"def train(model, ds):",
		"    for x, y in ds:",
		"        model(x)",
		"",
	}, "\n")
	writeFrameworkLeadFile(t, repoRoot, "b.py", sharedContent)
	writeFrameworkLeadFile(t, repoRoot, "a.py", sharedContent)

	extractor := newFrameworkLeadExtractor()
	leadPack, _, err := extractor.Extract(repoRoot)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(leadPack.Files) < 2 {
		t.Fatalf("expected at least two lead files, got %d", len(leadPack.Files))
	}
	if leadPack.Files[0].Path != "a.py" || leadPack.Files[1].Path != "b.py" {
		t.Fatalf("expected deterministic alphabetical ordering for equal scores, got %q then %q", leadPack.Files[0].Path, leadPack.Files[1].Path)
	}
}

func TestFrameworkLeadSummaryIncludesEvidenceSnippets(t *testing.T) {
	repoRoot := t.TempDir()
	snippet := "loader = DataLoader(ds)"
	writeFrameworkLeadFile(t, repoRoot, "train.py", strings.Join([]string{
		"import torch",
		"from torch.utils.data import DataLoader",
		"def train(model, ds):",
		"    " + snippet,
		"    for images, labels in loader:",
		"        model(images)",
		"",
	}, "\n"))

	extractor := newFrameworkLeadExtractor()
	_, summary, err := extractor.Extract(repoRoot)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(summary, snippet) {
		t.Fatalf("expected summary to include evidence snippet %q, got: %s", snippet, summary)
	}
}

func loadFrameworkLeadCases(t *testing.T) []frameworkLeadCase {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "framework_leads_cases.json"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var cases []frameworkLeadCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("expected non-empty framework lead cases")
	}
	return cases
}

func writeFrameworkLeadFile(t *testing.T, repoRoot, relativePath, contents string) {
	t.Helper()
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed for %q: %v", absolutePath, err)
	}
	if err := os.WriteFile(absolutePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile failed for %q: %v", absolutePath, err)
	}
}

