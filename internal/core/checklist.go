package core

import "sort"

// BuildVerifiedChecks computes checklist rows from explicit verification signals only.
func BuildVerifiedChecks(
	snapshot WorkspaceSnapshot,
	inspectIssues []Issue,
	validationIssues []Issue,
	blockerStepID EnsureStepID,
) []VerifiedCheck {
	issuesByStep := map[EnsureStepID][]Issue{}
	verifiedStepSet := map[EnsureStepID]struct{}{}

	for _, stepID := range inspectorVerifiedSteps(snapshot) {
		if !shouldRenderCheckStep(stepID) {
			continue
		}
		verifiedStepSet[stepID] = struct{}{}
	}
	if snapshot.Runtime.ProbeRan {
		verifiedStepSet[EnsureStepPythonRuntime] = struct{}{}
	}
	if snapshot.LeapCLI.ProbeRan {
		verifiedStepSet[EnsureStepLeapCLIAuth] = struct{}{}
		if snapshot.LeapCLI.Available {
			verifiedStepSet[EnsureStepServerConnectivity] = struct{}{}
		}
	}

	for _, issue := range mergeUniqueIssues(inspectIssues, validationIssues) {
		step := PreferredEnsureStepForIssue(issue)
		if step.ID == "" || !shouldRenderCheckStep(step.ID) {
			continue
		}
		verifiedStepSet[step.ID] = struct{}{}
		issuesByStep[step.ID] = append(issuesByStep[step.ID], issue)
	}

	checks := make([]VerifiedCheck, 0, len(verifiedStepSet))
	for _, step := range KnownEnsureSteps() {
		if _, ok := verifiedStepSet[step.ID]; !ok {
			continue
		}
		if !shouldRenderCheckStep(step.ID) {
			continue
		}

		issues := append([]Issue(nil), issuesByStep[step.ID]...)
		sortIssuesForChecklist(issues)

		status := CheckStatusPass
		for _, issue := range issues {
			if issue.Severity == SeverityError {
				status = CheckStatusFail
				break
			}
			if issue.Severity == SeverityWarning {
				status = CheckStatusWarning
			}
		}

		check := VerifiedCheck{
			StepID:   step.ID,
			Label:    HumanEnsureStepLabel(step.ID),
			Status:   status,
			Blocking: status == CheckStatusFail && blockerStepID != "" && step.ID == blockerStepID,
			Issues:   issues,
		}
		checks = append(checks, check)
	}

	return checks
}

func inspectorVerifiedSteps(snapshot WorkspaceSnapshot) []EnsureStepID {
	steps := []EnsureStepID{
		EnsureStepRepositoryContext,
		EnsureStepLeapYAML,
		EnsureStepIntegrationScript,
		EnsureStepIntegrationTestContract,
	}
	if _, ok := snapshot.FileHashes["leap.yaml"]; ok {
		steps = append(steps, EnsureStepModelAcquisition)
		steps = append(steps, EnsureStepModelContract)
	}
	return steps
}

// HasFailingVerifiedChecks reports whether any verified check failed.
func HasFailingVerifiedChecks(checks []VerifiedCheck) bool {
	for _, check := range checks {
		if check.Status == CheckStatusFail {
			return true
		}
	}
	return false
}

// HasWarningVerifiedChecks reports whether any verified check has warnings.
func HasWarningVerifiedChecks(checks []VerifiedCheck) bool {
	for _, check := range checks {
		if check.Status == CheckStatusWarning {
			return true
		}
	}
	return false
}

// VisibleChecksForFlow returns checks up to the first blocking issue; if no fail exists,
// it returns checks up to the first warning so output remains focused.
func VisibleChecksForFlow(checks []VerifiedCheck) []VerifiedCheck {
	if len(checks) == 0 {
		return nil
	}

	firstFail := -1
	firstWarning := -1
	for i, check := range checks {
		if check.Status == CheckStatusFail && firstFail < 0 {
			firstFail = i
		}
		if check.Status == CheckStatusWarning && firstWarning < 0 {
			firstWarning = i
		}
	}

	end := len(checks)
	if firstFail >= 0 {
		end = firstFail + 1
	} else if firstWarning >= 0 {
		end = firstWarning + 1
	}

	visible := make([]VerifiedCheck, end)
	copy(visible, checks[:end])
	return visible
}

// FirstAttentionCheck returns the first failing check, or first warning if there is no fail.
func FirstAttentionCheck(checks []VerifiedCheck) (VerifiedCheck, bool) {
	for _, check := range checks {
		if check.Status == CheckStatusFail {
			return check, true
		}
	}
	for _, check := range checks {
		if check.Status == CheckStatusWarning {
			return check, true
		}
	}
	return VerifiedCheck{}, false
}

func shouldRenderCheckStep(stepID EnsureStepID) bool {
	switch stepID {
	case EnsureStepComplete, EnsureStepInvestigate, EnsureStepUploadReadiness, EnsureStepUploadPush:
		return false
	default:
		return true
	}
}

func mergeUniqueIssues(sources ...[]Issue) []Issue {
	merged := make([]Issue, 0)
	seen := map[string]struct{}{}

	for _, source := range sources {
		for _, issue := range source {
			key := issueDedupKey(issue)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, issue)
		}
	}

	return merged
}

func issueDedupKey(issue Issue) string {
	key := string(issue.Code) + "|" + issue.Message + "|" + string(issue.Severity) + "|" + string(issue.Scope)
	if issue.Location == nil {
		return key
	}
	return key + "|" + issue.Location.Path + "|" + issue.Location.Symbol
}

func sortIssuesForChecklist(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return checklistSeverityRank(issues[i].Severity) > checklistSeverityRank(issues[j].Severity)
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Message < issues[j].Message
	})
}

func checklistSeverityRank(severity Severity) int {
	switch severity {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}
