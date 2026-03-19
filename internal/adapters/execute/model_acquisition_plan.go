package execute

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func selectedModelAcquisitionPlan(snapshot core.WorkspaceSnapshot, status core.IntegrationStatus) *core.ModelAcquisitionPlan {
	var source *core.ModelAcquisitionPlan
	if snapshot.ModelAcquisitionPlan != nil {
		source = snapshot.ModelAcquisitionPlan
	} else if status.Contracts != nil &&
		status.Contracts.ModelAcquisition != nil &&
		status.Contracts.ModelAcquisition.NormalizedPlan != nil {
		source = status.Contracts.ModelAcquisition.NormalizedPlan
	}
	if source == nil {
		return nil
	}

	plan := cloneExecuteModelAcquisitionPlan(source)
	if plan.SchemaVersion == "" {
		plan.SchemaVersion = "v1"
	}
	if strings.TrimSpace(plan.Strategy) == "" {
		plan.Strategy = "materialize_supported_artifact"
	}
	if plan.ExpectedOutputPath == "" {
		switch {
		case strings.TrimSpace(snapshot.SelectedModelPath) != "":
			plan.ExpectedOutputPath = normalizeModelAcquisitionPlanPath(snapshot.SelectedModelPath)
		case status.Contracts != nil && strings.TrimSpace(status.Contracts.ResolvedModelPath) != "":
			plan.ExpectedOutputPath = normalizeModelAcquisitionPlanPath(status.Contracts.ResolvedModelPath)
		default:
			plan.ExpectedOutputPath = normalizeModelAcquisitionPlanPath(defaultMaterializedModelPath(status))
		}
	} else {
		plan.ExpectedOutputPath = normalizeModelAcquisitionPlanPath(plan.ExpectedOutputPath)
	}
	plan.WorkingDir = normalizeModelAcquisitionPlanPath(plan.WorkingDir)
	plan.HelperPath = normalizeModelAcquisitionPlanPath(plan.HelperPath)
	return plan
}

func cloneExecuteModelAcquisitionPlan(plan *core.ModelAcquisitionPlan) *core.ModelAcquisitionPlan {
	if plan == nil {
		return nil
	}

	cloned := *plan
	if len(plan.RuntimeInvocation) > 0 {
		cloned.RuntimeInvocation = append([]string(nil), plan.RuntimeInvocation...)
	}
	if len(plan.Evidence) > 0 {
		cloned.Evidence = append([]core.ModelAcquisitionPlanEvidence(nil), plan.Evidence...)
	}
	return &cloned
}

func normalizeModelAcquisitionPlanPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func modelAcquisitionPlanConstraintEvidence(plan *core.ModelAcquisitionPlan) []string {
	if plan == nil || len(plan.Evidence) == 0 {
		return nil
	}

	lines := make([]string, 0, len(plan.Evidence))
	for _, item := range plan.Evidence {
		detail := strings.TrimSpace(item.Detail)
		if detail == "" {
			detail = "supporting evidence"
		}
		location := strings.TrimSpace(item.Path)
		if location != "" && item.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, item.Line)
		}
		snippet := strings.TrimSpace(item.Snippet)
		line := detail
		if location != "" {
			line = fmt.Sprintf("%s %s", location, line)
		}
		if snippet != "" {
			line = fmt.Sprintf("%s (%s)", line, snippet)
		}
		lines = append(lines, line)
	}
	return lines
}
