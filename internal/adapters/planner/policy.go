package planner

import (
	"sort"

	"github.com/tensorleap/concierge/internal/core"
)

type planningPolicy struct {
	priorityByStep map[core.EnsureStepID]int
}

func newPlanningPolicy() planningPolicy {
	priority := make(map[core.EnsureStepID]int, len(core.KnownEnsureSteps()))
	for index, step := range core.KnownEnsureSteps() {
		priority[step.ID] = index
	}
	return planningPolicy{priorityByStep: priority}
}

func (p planningPolicy) build(status core.IntegrationStatus) (core.EnsureStep, []core.EnsureStep, bool) {
	blockingIssues := collectBlockingIssues(status.Issues)
	if len(blockingIssues) == 0 {
		completeStep, _ := core.EnsureStepByID(core.EnsureStepComplete)
		return completeStep, nil, true
	}

	candidates := p.rankUniqueSteps(blockingIssues)
	if len(candidates) == 0 {
		completeStep, _ := core.EnsureStepByID(core.EnsureStepComplete)
		return completeStep, nil, true
	}

	primary := candidates[0]
	if primary.ID == core.EnsureStepUploadPush && !uploadReadinessClear(status.Issues) {
		fallback, ok := core.EnsureStepByID(core.EnsureStepUploadReadiness)
		if ok {
			primary = fallback
			if !containsStep(candidates, primary.ID) {
				candidates = append([]core.EnsureStep{primary}, candidates...)
			}
		}
	}

	additional := make([]core.EnsureStep, 0, len(candidates)-1)
	seen := map[core.EnsureStepID]struct{}{primary.ID: {}}
	for _, candidate := range candidates {
		if _, exists := seen[candidate.ID]; exists {
			continue
		}
		additional = append(additional, candidate)
		seen[candidate.ID] = struct{}{}
	}

	return primary, additional, false
}

func collectBlockingIssues(issues []core.Issue) []core.Issue {
	blocking := make([]core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == core.SeverityError {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func (p planningPolicy) rankUniqueSteps(issues []core.Issue) []core.EnsureStep {
	if len(issues) == 0 {
		return nil
	}

	type stepRank struct {
		step     core.EnsureStep
		severity int
		priority int
	}

	byStep := map[core.EnsureStepID]stepRank{}
	for _, issue := range issues {
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID == "" {
			continue
		}

		rank := stepRank{
			step:     step,
			severity: severityRank(issue.Severity),
			priority: p.stepPriority(step.ID),
		}

		existing, exists := byStep[step.ID]
		if !exists || rank.severity > existing.severity {
			byStep[step.ID] = rank
		}
	}

	ranks := make([]stepRank, 0, len(byStep))
	for _, rank := range byStep {
		ranks = append(ranks, rank)
	}

	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].severity != ranks[j].severity {
			return ranks[i].severity > ranks[j].severity
		}
		if ranks[i].priority != ranks[j].priority {
			return ranks[i].priority < ranks[j].priority
		}
		return ranks[i].step.ID < ranks[j].step.ID
	})

	steps := make([]core.EnsureStep, 0, len(ranks))
	for _, rank := range ranks {
		steps = append(steps, rank.step)
	}
	return steps
}

func (p planningPolicy) stepPriority(stepID core.EnsureStepID) int {
	if priority, ok := p.priorityByStep[stepID]; ok {
		return priority
	}
	return len(p.priorityByStep) + 1
}

func severityRank(severity core.Severity) int {
	switch severity {
	case core.SeverityError:
		return 3
	case core.SeverityWarning:
		return 2
	case core.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func uploadReadinessClear(issues []core.Issue) bool {
	for _, issue := range issues {
		step := core.PreferredEnsureStepForIssue(issue)
		if step.ID == core.EnsureStepUploadPush || step.ID == core.EnsureStepComplete {
			continue
		}
		if step.ID == core.EnsureStepInvestigate {
			return false
		}
		return false
	}
	return true
}

func containsStep(steps []core.EnsureStep, stepID core.EnsureStepID) bool {
	for _, step := range steps {
		if step.ID == stepID {
			return true
		}
	}
	return false
}
