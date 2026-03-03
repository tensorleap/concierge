package agent

import "github.com/tensorleap/concierge/internal/core"

// AgentTask describes one task-scoped objective delegated to a coding agent.
type AgentTask struct {
	Objective      string   `json:"objective"`
	Constraints    []string `json:"constraints,omitempty"`
	RepoRoot       string   `json:"repoRoot"`
	TranscriptPath string   `json:"transcriptPath"`
}

// AgentResult captures the outcome of an agent task execution.
type AgentResult struct {
	Applied        bool                `json:"applied"`
	TranscriptPath string              `json:"transcriptPath,omitempty"`
	Summary        string              `json:"summary,omitempty"`
	Evidence       []core.EvidenceItem `json:"evidence,omitempty"`
}
