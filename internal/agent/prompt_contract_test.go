package agent

import (
	"strings"
	"testing"

	"github.com/tensorleap/concierge/internal/core"
)

func TestBuildClaudeTaskPromptIncludesAllRequiredSections(t *testing.T) {
	task := AgentTask{
		Objective: "Implement missing preprocess contract behavior",
		ScopePolicy: &AgentScopePolicy{
			AllowedFiles:       []string{"leap_integration.py"},
			ForbiddenAreas:     []string{"Do not touch training loop"},
			StopAndAskTriggers: []string{"Missing model path evidence"},
		},
		RepoContext: &core.AgentRepoContext{
			RepoRoot:              "/tmp/repo",
			EntryFile:             "leap_integration.py",
			LeapYAMLBoundary:      "leap.yaml present",
			RuntimeKind:           "poetry",
			RuntimeInterpreter:    "/tmp/repo/.venv/bin/python",
			RuntimeStatus:         "dependencies ready; code_loader import succeeded (v1.0.166)",
			SelectedModelPath:     "models/model.onnx",
			ModelCandidates:       []string{"models/model.onnx"},
			DecoratorInventory:    []string{"preprocess:build_preprocess"},
			BlockingIssues:        []string{"preprocess_function_missing"},
		},
		DomainKnowledge: &AgentDomainKnowledgePack{
			Version:    "tlkp-v1",
			SectionIDs: []string{"preprocess_contract", "load_model_contract"},
			Sections: map[string]string{
				"preprocess_contract": "Preprocess must produce train and validation subsets.",
				"load_model_contract": "Load model decorators must target .onnx or .h5 files.",
			},
		},
		AcceptanceChecks: []string{"Train and validation subsets are both wired"},
	}

	prompt := BuildClaudeTaskPrompt(task)

	requiredSections := []string{
		"Objective:",
		"Edit Scope:",
		"Repository Facts:",
		"Tensorleap Rules:",
		"Acceptance Checks:",
	}
	lastIndex := -1
	for _, section := range requiredSections {
		index := strings.Index(prompt, section)
		if index < 0 {
			t.Fatalf("expected section %q in prompt, got: %q", section, prompt)
		}
		if index <= lastIndex {
			t.Fatalf("expected section %q to appear after prior sections, got prompt: %q", section, prompt)
		}
		lastIndex = index
	}

	if !strings.Contains(prompt, "Knowledge pack version: tlkp-v1") {
		t.Fatalf("expected knowledge pack version metadata in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "Prepared runtime: poetry") {
		t.Fatalf("expected runtime kind metadata in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "Runtime interpreter: /tmp/repo/.venv/bin/python") {
		t.Fatalf("expected runtime interpreter metadata in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "Runtime status: dependencies ready; code_loader import succeeded (v1.0.166)") {
		t.Fatalf("expected runtime status metadata in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "[preprocess_contract]") || !strings.Contains(prompt, "[load_model_contract]") {
		t.Fatalf("expected requested Tensorleap rule sections in prompt, got: %q", prompt)
	}
}

func TestBuildClaudeTaskPromptForInputStepExcludesOutOfScopeRuleSections(t *testing.T) {
	task := AgentTask{
		Objective: "Implement missing input encoders",
		ScopePolicy: &AgentScopePolicy{
			AllowedFiles: []string{"leap_integration.py"},
		},
		RepoContext: &core.AgentRepoContext{
			RepoRoot:  "/tmp/repo",
			EntryFile: "leap_integration.py",
		},
		DomainKnowledge: &AgentDomainKnowledgePack{
			Version:    "tlkp-v1",
			SectionIDs: []string{"input_encoder_contract"},
			Sections: map[string]string{
				"input_encoder_contract":        "Input encoders must preserve expected shape/dtype contracts.",
				"ground_truth_encoder_contract": "GT encoders run only on labeled subsets.",
			},
		},
		AcceptanceChecks: []string{"All required input symbols have encoders"},
	}

	prompt := BuildClaudeTaskPrompt(task)

	if !strings.Contains(prompt, "[input_encoder_contract]") {
		t.Fatalf("expected input-encoder section in prompt, got: %q", prompt)
	}
	if strings.Contains(prompt, "[ground_truth_encoder_contract]") {
		t.Fatalf("did not expect out-of-scope GT section in prompt, got: %q", prompt)
	}
	if strings.Contains(prompt, "GT encoders run only on labeled subsets.") {
		t.Fatalf("did not expect out-of-scope GT rule text in prompt, got: %q", prompt)
	}
}
