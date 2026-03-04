package agent

import "github.com/tensorleap/concierge/internal/core"

// AgentTask describes one task-scoped objective delegated to a coding agent.
type AgentTask struct {
	Objective        string                    `json:"objective"`
	Constraints      []string                  `json:"constraints,omitempty"`
	AcceptanceChecks []string                  `json:"acceptanceChecks,omitempty"`
	ScopePolicy      *AgentScopePolicy         `json:"scopePolicy,omitempty"`
	RepoContext      *core.AgentRepoContext    `json:"repoContext,omitempty"`
	DomainKnowledge  *AgentDomainKnowledgePack `json:"domainKnowledge,omitempty"`
	RepoRoot         string                    `json:"repoRoot"`
	TranscriptPath   string                    `json:"transcriptPath"`
}

// AgentScopePolicy defines explicit edit boundaries and Tensorleap rule slices for one task.
type AgentScopePolicy struct {
	AllowedFiles       []string `json:"allowedFiles,omitempty"`
	ForbiddenAreas     []string `json:"forbiddenAreas,omitempty"`
	RequiredOutcomes   []string `json:"requiredOutcomes,omitempty"`
	StopAndAskTriggers []string `json:"stopAndAskTriggers,omitempty"`
	DomainSections     []string `json:"domainSections,omitempty"`
}

// KnowledgeSource captures provenance metadata for one Tensorleap rule section.
type KnowledgeSource struct {
	Section        string `json:"section" yaml:"section"`
	SectionLabel   string `json:"sectionLabel" yaml:"section_label"`
	SourceURL      string `json:"sourceUrl" yaml:"source_url"`
	LastReviewedAt string `json:"lastReviewedAt" yaml:"last_reviewed_at"`
}

// DomainKnowledgePack contains versioned Tensorleap rule sections and source metadata.
type DomainKnowledgePack struct {
	Version  string            `json:"version"`
	Sections map[string]string `json:"sections"`
	Sources  []KnowledgeSource `json:"sources"`
}

// AgentDomainKnowledgePack captures the scoped Tensorleap rules attached to one task.
type AgentDomainKnowledgePack struct {
	Version    string            `json:"version"`
	SectionIDs []string          `json:"sectionIds,omitempty"`
	Sections   map[string]string `json:"sections,omitempty"`
}

// AgentResult captures the outcome of an agent task execution.
type AgentResult struct {
	Applied        bool                `json:"applied"`
	TranscriptPath string              `json:"transcriptPath,omitempty"`
	Summary        string              `json:"summary,omitempty"`
	Evidence       []core.EvidenceItem `json:"evidence,omitempty"`
}
