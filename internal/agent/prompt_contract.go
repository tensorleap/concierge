package agent

import (
	"fmt"
	"strings"
)

const claudeSystemPrompt = `
You are a task-scoped coding collaborator running under Concierge.
Concierge is the deterministic orchestrator; you are not the orchestrator.

Operating responsibilities:
- Complete only the specific objective provided for this task.
- Keep edits minimal, local, and reviewable.
- Prioritize Tensorleap integration files and avoid unrelated repository changes.
- Do not refactor or modify unrelated user/training/business logic.
- Preserve existing behavior outside the requested scope.
- Follow Tensorleap contract rules exactly as provided in the task prompt.
- If repository state is ambiguous or objective conflicts appear, stop and state the blocker clearly.
- Never run git commit/push/rebase/reset operations.
- Never access files outside the repository root unless explicitly instructed.
`

// BuildClaudeSystemPrompt returns the stable global operating policy for Claude invocations.
func BuildClaudeSystemPrompt() string {
	return strings.TrimSpace(claudeSystemPrompt)
}

// BuildClaudeTaskPrompt renders the deterministic structured user prompt for one task.
func BuildClaudeTaskPrompt(task AgentTask) string {
	var b strings.Builder

	b.WriteString("Objective:\n")
	b.WriteString(nonEmptyOrFallback(task.Objective, "<missing objective>"))
	b.WriteString("\n\n")

	b.WriteString("Edit Scope:\n")
	b.WriteString("Allowed files:\n")
	b.WriteString(renderBulletList(scopeValues(task.ScopePolicy, "allowed")))
	b.WriteString("Forbidden areas:\n")
	b.WriteString(renderBulletList(scopeValues(task.ScopePolicy, "forbidden")))
	b.WriteString("Stop-and-ask triggers:\n")
	b.WriteString(renderBulletList(scopeValues(task.ScopePolicy, "stop_and_ask")))
	b.WriteString("\n")

	b.WriteString("Repository Facts:\n")
	b.WriteString(fmt.Sprintf("- Repo root: %s\n", firstNonEmpty(
		repoContextValue(task, "repo_root"),
		task.RepoRoot,
		"<unknown>",
	)))
	b.WriteString(fmt.Sprintf("- Entry file: %s\n", nonEmptyOrFallback(repoContextValue(task, "entry_file"), "<unknown>")))
	b.WriteString(fmt.Sprintf("- Binder file: %s\n", nonEmptyOrFallback(repoContextValue(task, "binder_file"), "<unknown>")))
	b.WriteString(fmt.Sprintf("- leap.yaml boundary: %s\n", nonEmptyOrFallback(repoContextValue(task, "leap_yaml_boundary"), "<unknown>")))
	b.WriteString(fmt.Sprintf("- Selected model path: %s\n", nonEmptyOrFallback(repoContextValue(task, "selected_model_path"), "<none>")))
	b.WriteString(fmt.Sprintf("- Model candidates: %s\n", renderInlineList(repoContextValues(task, "model_candidates"))))
	b.WriteString(fmt.Sprintf("- Decorator inventory: %s\n", renderInlineList(repoContextValues(task, "decorator_inventory"))))
	b.WriteString(fmt.Sprintf("- Integration-test calls: %s\n", renderInlineList(repoContextValues(task, "integration_test_calls"))))
	b.WriteString(fmt.Sprintf("- Blocking issues: %s\n", renderInlineList(repoContextValues(task, "blocking_issues"))))
	b.WriteString(fmt.Sprintf("- Validation findings: %s\n", renderInlineList(repoContextValues(task, "validation_findings"))))
	b.WriteString("\n")

	b.WriteString("Tensorleap Rules:\n")
	knowledge := task.DomainKnowledge
	if knowledge == nil {
		b.WriteString("- Knowledge pack version: <missing>\n")
		b.WriteString("- Rule sections: <none>\n\n")
	} else {
		b.WriteString(fmt.Sprintf("- Knowledge pack version: %s\n", nonEmptyOrFallback(knowledge.Version, "<missing>")))
		sectionIDs := normalizedUniqueOrdered(knowledge.SectionIDs)
		b.WriteString(fmt.Sprintf("- Rule sections: %s\n", renderInlineList(sectionIDs)))
		if len(sectionIDs) > 0 {
			b.WriteString("\n")
		}
		for _, sectionID := range sectionIDs {
			body := strings.TrimSpace(knowledge.Sections[sectionID])
			if body == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("[%s]\n", sectionID))
			b.WriteString(body)
			b.WriteString("\n\n")
		}
	}

	b.WriteString("Acceptance Checks:\n")
	b.WriteString(renderBulletList(taskAcceptanceChecks(task)))

	return strings.TrimSpace(b.String())
}

func taskAcceptanceChecks(task AgentTask) []string {
	checks := make([]string, 0, len(task.AcceptanceChecks)+len(task.Constraints))
	checks = append(checks, task.AcceptanceChecks...)
	checks = append(checks, task.Constraints...)
	return normalizedUniqueOrdered(checks)
}

func scopeValues(policy *AgentScopePolicy, key string) []string {
	if policy == nil {
		return nil
	}
	switch key {
	case "allowed":
		return policy.AllowedFiles
	case "forbidden":
		return policy.ForbiddenAreas
	case "stop_and_ask":
		return policy.StopAndAskTriggers
	default:
		return nil
	}
}

func repoContextValue(task AgentTask, key string) string {
	if task.RepoContext == nil {
		return ""
	}
	switch key {
	case "repo_root":
		return task.RepoContext.RepoRoot
	case "entry_file":
		return task.RepoContext.EntryFile
	case "binder_file":
		return task.RepoContext.BinderFile
	case "leap_yaml_boundary":
		return task.RepoContext.LeapYAMLBoundary
	case "selected_model_path":
		return task.RepoContext.SelectedModelPath
	default:
		return ""
	}
}

func repoContextValues(task AgentTask, key string) []string {
	if task.RepoContext == nil {
		return nil
	}
	switch key {
	case "model_candidates":
		return task.RepoContext.ModelCandidates
	case "decorator_inventory":
		return task.RepoContext.DecoratorInventory
	case "integration_test_calls":
		return task.RepoContext.IntegrationTestCalls
	case "blocking_issues":
		return task.RepoContext.BlockingIssues
	case "validation_findings":
		return task.RepoContext.ValidationFindings
	default:
		return nil
	}
}

func renderBulletList(values []string) string {
	items := normalizedUniqueOrdered(values)
	if len(items) == 0 {
		return "- <none>\n"
	}

	var b strings.Builder
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return b.String()
}

func renderInlineList(values []string) string {
	items := normalizedUniqueOrdered(values)
	if len(items) == 0 {
		return "<none>"
	}
	return strings.Join(items, ", ")
}

func normalizedUniqueOrdered(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func nonEmptyOrFallback(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		return trimmed
	}
	return ""
}
