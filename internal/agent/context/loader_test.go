package context

import (
	"strings"
	"testing"
)

func TestLoadDomainKnowledgePackSuccess(t *testing.T) {
	pack, err := LoadDomainKnowledgePack()
	if err != nil {
		t.Fatalf("LoadDomainKnowledgePack returned error: %v", err)
	}

	if pack.Version != "tlkp-v1" {
		t.Fatalf("expected version %q, got %q", "tlkp-v1", pack.Version)
	}

	for _, required := range requiredKnowledgeSections {
		body := strings.TrimSpace(pack.Sections[required])
		if body == "" {
			t.Fatalf("expected required section %q to be present", required)
		}
	}

	if len(pack.Sources) < len(requiredKnowledgeSections) {
		t.Fatalf("expected at least %d source entries, got %d", len(requiredKnowledgeSections), len(pack.Sources))
	}
}

func TestLoadDomainKnowledgePackRejectsMissingRequiredSections(t *testing.T) {
	knowledgeMarkdown := []byte(`
# Tensorleap Knowledge Pack
version: tlkp-v1

## leap_yaml_contract
- leap.yaml is required.
`)

	knowledgeSources := []byte(`
version: tlkp-v1
sources:
  - section: leap_yaml_contract
    section_label: leap yaml rules
    source_url: https://docs.tensorleap.ai/tensorleap-integration/leap.yaml
    last_reviewed_at: "2026-03-03"
`)

	_, err := buildDomainKnowledgePack(knowledgeMarkdown, knowledgeSources)
	if err == nil {
		t.Fatal("expected error when required sections are missing")
	}
	if !strings.Contains(err.Error(), "knowledge pack missing required section(s)") {
		t.Fatalf("expected required-sections error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "preprocess_contract") {
		t.Fatalf("expected missing preprocess section in error, got: %v", err)
	}
}

func TestLoadDomainKnowledgePackParsesSourceManifest(t *testing.T) {
	rawManifest := []byte(`
version: tlkp-v1
sources:
  - section: leap_yaml_contract
    section_label: leap yaml rules
    source_url: https://docs.tensorleap.ai/tensorleap-integration/leap.yaml
    last_reviewed_at: "2026-03-03"
`)

	manifest, err := parseSourceManifest(rawManifest)
	if err != nil {
		t.Fatalf("parseSourceManifest returned error: %v", err)
	}
	if manifest.Version != "tlkp-v1" {
		t.Fatalf("expected version %q, got %q", "tlkp-v1", manifest.Version)
	}
	if len(manifest.Sources) != 1 {
		t.Fatalf("expected 1 source entry, got %d", len(manifest.Sources))
	}

	source := manifest.Sources[0]
	if source.Section != "leap_yaml_contract" {
		t.Fatalf("expected section %q, got %q", "leap_yaml_contract", source.Section)
	}
	if source.SectionLabel != "leap yaml rules" {
		t.Fatalf("expected section label %q, got %q", "leap yaml rules", source.SectionLabel)
	}
	if source.SourceURL == "" || source.LastReviewedAt == "" {
		t.Fatalf("expected source metadata fields to be populated, got %+v", source)
	}
}
