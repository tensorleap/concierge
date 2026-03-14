package fixtures

import (
	"os"
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/agent"
	"github.com/tensorleap/concierge/internal/core"
)

func TestFixtureCaseAgentContextQuality(t *testing.T) {
	requireFixtureCaseReposPrepared(t)

	testCases := []struct {
		name               string
		caseID             string
		stepID             core.EnsureStepID
		expectedSections   []string
		absentSections     []string
		requiredConstraint string
		caseConstraint     func(fixtureCaseEntry) string
		requiredForbidden  string
	}{
		{
			name:               "model",
			caseID:             "mnist_load_model",
			stepID:             core.EnsureStepModelContract,
			expectedSections:   []string{"load_model_contract"},
			absentSections:     []string{"input_encoder_contract", "ground_truth_encoder_contract", "integration_test_wiring_contract"},
			requiredConstraint: "Candidate model paths:",
			requiredForbidden:  "Do not modify @tensorleap_preprocess definitions or subset semantics in this step",
		},
		{
			name:               "preprocess",
			caseID:             "mnist_missing_preprocess",
			stepID:             core.EnsureStepPreprocessContract,
			expectedSections:   []string{"preprocess_contract", "load_model_contract"},
			absentSections:     []string{"ground_truth_encoder_contract", "integration_test_wiring_contract"},
			requiredConstraint: "Implement a preprocess function that returns both train and validation subsets.",
			requiredForbidden:  "Do not modify @tensorleap_input_encoder definitions or registrations",
		},
		{
			name:               "input_encoders",
			caseID:             "mnist_minimum_inputs",
			stepID:             core.EnsureStepInputEncoders,
			expectedSections:   []string{"input_encoder_contract"},
			absentSections:     []string{"ground_truth_encoder_contract", "integration_test_wiring_contract", "load_model_contract"},
			requiredConstraint: "Required input symbols: meta",
			requiredForbidden:  "Do not modify @tensorleap_gt_encoder definitions or registrations",
		},
		{
			name:               "gt_encoders",
			caseID:             "mnist_gt_encoders",
			stepID:             core.EnsureStepGroundTruthEncoders,
			expectedSections:   []string{"ground_truth_encoder_contract"},
			absentSections:     []string{"input_encoder_contract", "integration_test_wiring_contract", "load_model_contract"},
			requiredConstraint: "Required ground-truth symbols: label",
			requiredForbidden:  "Do not modify @tensorleap_input_encoder definitions or registrations",
		},
		{
			name:             "integration_test",
			caseID:           "mnist_integration_test_wiring",
			stepID:           core.EnsureStepIntegrationTestContract,
			expectedSections: []string{"integration_test_wiring_contract"},
			absentSections:   []string{"input_encoder_contract", "ground_truth_encoder_contract", "load_model_contract"},
			caseConstraint: func(entry fixtureCaseEntry) string {
				return entry.ExpectedMissingIntegrationCall
			},
			requiredForbidden: "Do not modify @tensorleap_preprocess subset semantics in this step",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			entry, repoRoot := cloneCaseRepoForTest(t, tc.caseID)
			runner, result := executeAgentStepForCase(t, entry, repoRoot, tc.stepID)

			if runner.lastTask.ScopePolicy == nil {
				t.Fatal("expected scope policy on prepared agent task")
			}
			if runner.lastTask.RepoContext == nil {
				t.Fatal("expected repo context on prepared agent task")
			}
			if runner.lastTask.DomainKnowledge == nil {
				t.Fatal("expected domain knowledge on prepared agent task")
			}

			prompt := agent.BuildClaudeTaskPrompt(runner.lastTask)
			for _, sectionID := range tc.expectedSections {
				if !containsString(runner.lastTask.ScopePolicy.DomainSections, sectionID) {
					t.Fatalf("expected domain sections %+v to include %q", runner.lastTask.ScopePolicy.DomainSections, sectionID)
				}
				assertPromptSectionPresent(t, prompt, sectionID)
			}
			for _, sectionID := range tc.absentSections {
				assertPromptSectionAbsent(t, prompt, sectionID)
			}

			if strings.TrimSpace(tc.requiredConstraint) != "" {
				assertConstraintContains(t, runner.lastTask, tc.requiredConstraint)
			}
			if tc.caseConstraint != nil {
				assertConstraintContains(t, runner.lastTask, tc.caseConstraint(entry))
			}
			if !containsString(runner.lastTask.ScopePolicy.ForbiddenAreas, tc.requiredForbidden) {
				t.Fatalf("expected forbidden areas %+v to include %q", runner.lastTask.ScopePolicy.ForbiddenAreas, tc.requiredForbidden)
			}
			if strings.TrimSpace(evidenceValue(result.Evidence, "agent.repo_context.path")) == "" {
				t.Fatalf("expected repo-context evidence path, got %+v", result.Evidence)
			}
			if _, err := os.Stat(evidenceValue(result.Evidence, "agent.repo_context.path")); err != nil {
				t.Fatalf("expected repo-context evidence file to exist: %v", err)
			}
			if evidenceValue(result.Evidence, "agent.scope_policy.domain_sections") == "" {
				t.Fatalf("expected scope-policy evidence, got %+v", result.Evidence)
			}
		})
	}
}
