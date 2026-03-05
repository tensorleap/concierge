package fixtures

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/adapters/validate"
)

func TestFixtureDiscoveryParity_ResearchTargets_PreVariants(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	testCases := []struct {
		id            string
		requireInputs bool
		requireGT     bool
	}{
		{id: "yolov5_visdrone", requireInputs: true, requireGT: true},
		{id: "ultralytics", requireInputs: true, requireGT: true},
		{id: "imdb", requireInputs: false, requireGT: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			preRoot, _ := resolveFixtureRoots(t, tc.id)
			status := inspectStatus(t, preRoot)

			if status.Contracts == nil {
				t.Fatalf("expected contracts for fixture %q pre variant", tc.id)
			}
			if status.Contracts.InputGTDiscovery == nil {
				t.Fatalf("expected input/GT discovery artifacts for fixture %q pre variant", tc.id)
			}
			artifacts := status.Contracts.InputGTDiscovery
			if artifacts.LeadPack == nil || len(artifacts.LeadPack.Files) == 0 {
				t.Fatalf("expected non-empty lead pack for fixture %q pre variant, got %+v", tc.id, artifacts.LeadPack)
			}
			if strings.TrimSpace(artifacts.LeadSummary) == "" {
				t.Fatalf("expected lead summary for fixture %q pre variant", tc.id)
			}
			if artifacts.AgentPromptBundle == nil || strings.TrimSpace(artifacts.AgentPromptBundle.UserPrompt) == "" {
				t.Fatalf("expected prompt bundle for fixture %q pre variant", tc.id)
			}
			if artifacts.AgentRawOutput == nil || strings.TrimSpace(artifacts.AgentRawOutput.Payload) == "" {
				t.Fatalf("expected raw investigator payload for fixture %q pre variant", tc.id)
			}
			if artifacts.NormalizedFindings == nil {
				t.Fatalf("expected normalized findings for fixture %q pre variant", tc.id)
			}
			if artifacts.ComparisonReport == nil {
				t.Fatalf("expected comparison report for fixture %q pre variant", tc.id)
			}
			if tc.requireInputs && len(status.Contracts.DiscoveredInputSymbols) == 0 {
				t.Fatalf("expected discovered input symbols for fixture %q pre variant", tc.id)
			}
			if tc.requireGT && len(status.Contracts.DiscoveredGroundTruthSymbols) == 0 {
				t.Fatalf("expected discovered ground-truth symbols for fixture %q pre variant", tc.id)
			}
			if tc.id == "imdb" && len(status.Contracts.DiscoveredInputSymbols) == 0 && len(status.Contracts.DiscoveredGroundTruthSymbols) == 0 {
				if len(artifacts.NormalizedFindings.Unknowns) == 0 {
					t.Fatalf("expected imdb edge-case unknowns when no candidates are discovered")
				}
			}
		})
	}
}

func TestFixtureDiscoveryParity_RuntimeSignatureNotesPresent(t *testing.T) {
	requireFixtureReposPrepared(t)
	t.Setenv(validate.HarnessEnableEnvVar, "0")

	for _, fixtureID := range []string{"yolov5_visdrone", "ultralytics", "imdb"} {
		fixtureID := fixtureID
		t.Run(fixtureID, func(t *testing.T) {
			_, postRoot := resolveFixtureRoots(t, fixtureID)
			status := inspectStatus(t, postRoot)
			if status.Contracts == nil || status.Contracts.InputGTDiscovery == nil || status.Contracts.InputGTDiscovery.ComparisonReport == nil {
				t.Fatalf("expected discovery comparison report for fixture %q post variant", fixtureID)
			}

			notes := status.Contracts.InputGTDiscovery.ComparisonReport.Notes
			if !containsPrefix(notes, "runtime_signature:") {
				t.Fatalf("expected runtime signature note for fixture %q post variant, got %+v", fixtureID, notes)
			}
		})
	}
}

func containsPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(strings.TrimSpace(value), prefix) {
			return true
		}
	}
	return false
}
