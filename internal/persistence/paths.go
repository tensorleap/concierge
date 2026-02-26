package persistence

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// Paths builds deterministic paths under .concierge.
type Paths struct {
	projectRoot string
}

// NewPaths validates and stores the project root for .concierge artifacts.
func NewPaths(projectRoot string) (*Paths, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, core.NewError(core.KindUnknown, "persistence.paths.new", "project root is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, core.WrapError(core.KindUnknown, "persistence.paths.abs", err)
	}

	return &Paths{projectRoot: absRoot}, nil
}

// ProjectRoot returns the absolute project root configured for this layout.
func (p *Paths) ProjectRoot() string {
	if p == nil {
		return ""
	}
	return p.projectRoot
}

// ConciergeRoot returns the .concierge root directory path.
func (p *Paths) ConciergeRoot() string {
	return filepath.Join(p.ProjectRoot(), ".concierge")
}

// StateDir returns the state directory path.
func (p *Paths) StateDir() string {
	return filepath.Join(p.ConciergeRoot(), "state")
}

// StateFile returns the state file path.
func (p *Paths) StateFile() string {
	return filepath.Join(p.StateDir(), "state.json")
}

// ReportsDir returns the reports directory path.
func (p *Paths) ReportsDir() string {
	return filepath.Join(p.ConciergeRoot(), "reports")
}

// ReportFile returns the report path for a snapshot ID.
func (p *Paths) ReportFile(snapshotID string) string {
	fileName := sanitizePathToken(snapshotID)
	return filepath.Join(p.ReportsDir(), fmt.Sprintf("%s.json", fileName))
}

// EvidenceDir returns the evidence directory path for a snapshot ID.
func (p *Paths) EvidenceDir(snapshotID string) string {
	return filepath.Join(p.ConciergeRoot(), "evidence", sanitizePathToken(snapshotID))
}

// EvidenceFile returns the evidence file path for a snapshot ID and evidence item name.
func (p *Paths) EvidenceFile(snapshotID, evidenceName string) string {
	fileName := sanitizePathToken(evidenceName)
	return filepath.Join(p.EvidenceDir(snapshotID), fmt.Sprintf("%s.log", fileName))
}

func sanitizePathToken(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "unknown"
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	result := strings.Trim(b.String(), "._-")
	if result == "" {
		return "unknown"
	}
	return result
}
