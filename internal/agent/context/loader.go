package context

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
	"gopkg.in/yaml.v3"
)

const (
	knowledgeMarkdownPath = "tensorleap_knowledge_v1.md"
	knowledgeSourcesPath  = "tensorleap_knowledge_sources.yaml"
)

var requiredKnowledgeSections = []string{
	"leap_yaml_contract",
	"preprocess_contract",
	"input_encoder_contract",
	"ground_truth_encoder_contract",
	"integration_test_wiring_contract",
	"load_model_contract",
}

type sourceManifest struct {
	Version string                  `yaml:"version"`
	Sources []agent.KnowledgeSource `yaml:"sources"`
}

//go:embed tensorleap_knowledge_v1.md tensorleap_knowledge_sources.yaml
var knowledgeFiles embed.FS

// LoadDomainKnowledgePack loads and validates the checked-in Tensorleap knowledge pack files.
func LoadDomainKnowledgePack() (agent.DomainKnowledgePack, error) {
	knowledgeMarkdown, err := knowledgeFiles.ReadFile(knowledgeMarkdownPath)
	if err != nil {
		return agent.DomainKnowledgePack{}, core.WrapError(core.KindUnknown, "agent.context.knowledge_read", err)
	}

	knowledgeSources, err := knowledgeFiles.ReadFile(knowledgeSourcesPath)
	if err != nil {
		return agent.DomainKnowledgePack{}, core.WrapError(core.KindUnknown, "agent.context.sources_read", err)
	}

	pack, err := buildDomainKnowledgePack(knowledgeMarkdown, knowledgeSources)
	if err != nil {
		return agent.DomainKnowledgePack{}, core.WrapError(core.KindUnknown, "agent.context.load", err)
	}
	return pack, nil
}

func buildDomainKnowledgePack(knowledgeMarkdown []byte, knowledgeSources []byte) (agent.DomainKnowledgePack, error) {
	version, err := parseKnowledgeVersion(string(knowledgeMarkdown))
	if err != nil {
		return agent.DomainKnowledgePack{}, fmt.Errorf("parse knowledge version: %w", err)
	}

	sections, err := parseKnowledgeSections(string(knowledgeMarkdown))
	if err != nil {
		return agent.DomainKnowledgePack{}, fmt.Errorf("parse knowledge sections: %w", err)
	}

	if err := validateRequiredSections(sections); err != nil {
		return agent.DomainKnowledgePack{}, err
	}

	manifest, err := parseSourceManifest(knowledgeSources)
	if err != nil {
		return agent.DomainKnowledgePack{}, fmt.Errorf("parse source manifest: %w", err)
	}

	if manifest.Version != version {
		return agent.DomainKnowledgePack{}, fmt.Errorf(
			"knowledge version %q does not match source manifest version %q",
			version,
			manifest.Version,
		)
	}

	if err := validateSources(manifest.Sources, sections); err != nil {
		return agent.DomainKnowledgePack{}, err
	}

	return agent.DomainKnowledgePack{
		Version:  version,
		Sections: sections,
		Sources:  manifest.Sources,
	}, nil
}

func parseKnowledgeVersion(markdown string) (string, error) {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "version:") {
			continue
		}
		version := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
		if version == "" {
			return "", fmt.Errorf("version is empty")
		}
		return version, nil
	}
	return "", fmt.Errorf("version line is missing")
}

func parseKnowledgeSections(markdown string) (map[string]string, error) {
	lines := strings.Split(markdown, "\n")
	sections := map[string]string{}

	currentSection := ""
	var body strings.Builder

	flush := func() error {
		if currentSection == "" {
			return nil
		}
		sectionBody := strings.TrimSpace(body.String())
		if sectionBody == "" {
			return fmt.Errorf("section %q is empty", currentSection)
		}
		sections[currentSection] = sectionBody
		return nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if err := flush(); err != nil {
				return nil, err
			}

			sectionID := normalizeSectionID(strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
			if sectionID == "" {
				return nil, fmt.Errorf("found section heading without identifier")
			}
			if _, exists := sections[sectionID]; exists {
				return nil, fmt.Errorf("section %q is defined more than once", sectionID)
			}

			currentSection = sectionID
			body.Reset()
			continue
		}

		if currentSection == "" {
			continue
		}
		body.WriteString(line)
		body.WriteString("\n")
	}

	if err := flush(); err != nil {
		return nil, err
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("no sections were discovered")
	}

	return sections, nil
}

func normalizeSectionID(sectionID string) string {
	normalized := strings.TrimSpace(strings.ToLower(sectionID))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

func validateRequiredSections(sections map[string]string) error {
	var missing []string
	for _, section := range requiredKnowledgeSections {
		body := strings.TrimSpace(sections[section])
		if body == "" {
			missing = append(missing, section)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("knowledge pack missing required section(s): %s", strings.Join(missing, ", "))
}

func parseSourceManifest(raw []byte) (sourceManifest, error) {
	var manifest sourceManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return sourceManifest{}, err
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		return sourceManifest{}, fmt.Errorf("version is required")
	}
	if len(manifest.Sources) == 0 {
		return sourceManifest{}, fmt.Errorf("at least one source entry is required")
	}
	return manifest, nil
}

func validateSources(sources []agent.KnowledgeSource, sections map[string]string) error {
	coverage := map[string]int{}
	for index, source := range sources {
		section := normalizeSectionID(source.Section)
		if section == "" {
			return fmt.Errorf("source[%d] section is required", index)
		}
		if strings.TrimSpace(source.SectionLabel) == "" {
			return fmt.Errorf("source[%d] section_label is required", index)
		}
		if strings.TrimSpace(source.SourceURL) == "" {
			return fmt.Errorf("source[%d] source_url is required", index)
		}
		lastReviewedAt := strings.TrimSpace(source.LastReviewedAt)
		if lastReviewedAt == "" {
			return fmt.Errorf("source[%d] last_reviewed_at is required", index)
		}
		if _, err := time.Parse("2006-01-02", lastReviewedAt); err != nil {
			return fmt.Errorf("source[%d] last_reviewed_at must be YYYY-MM-DD: %w", index, err)
		}
		if _, known := sections[section]; !known {
			return fmt.Errorf("source[%d] references unknown section %q", index, source.Section)
		}
		coverage[section]++
	}

	uncovered := make([]string, 0)
	for section := range sections {
		if coverage[section] > 0 {
			continue
		}
		uncovered = append(uncovered, section)
	}
	sort.Strings(uncovered)
	if len(uncovered) > 0 {
		return fmt.Errorf("source manifest is missing entries for section(s): %s", strings.Join(uncovered, ", "))
	}

	return nil
}
