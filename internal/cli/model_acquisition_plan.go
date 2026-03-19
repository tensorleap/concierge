package cli

import (
	"strings"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/state"
)

func modelAcquisitionPlanFromSelection(
	selectedModelPath string,
	clarification *state.ModelAcquisitionClarification,
) *core.ModelAcquisitionPlan {
	if clarification == nil {
		return nil
	}

	if selectedVerifiedPath := normalizeModelPathValue(clarification.SelectedVerifiedModelPath); selectedVerifiedPath != "" {
		return &core.ModelAcquisitionPlan{
			SchemaVersion:      "v1",
			CanMaterialize:     true,
			DefaultChoice:      selectedVerifiedPath,
			Strategy:           "selected_verified_artifact",
			ExpectedOutputPath: selectedVerifiedPath,
			Confidence:         "user_confirmed",
			Evidence: []core.ModelAcquisitionPlanEvidence{
				{
					Detail:  "user selected verified model artifact",
					Snippet: selectedVerifiedPath,
				},
			},
		}
	}

	note := strings.TrimSpace(clarification.ModelSourceNote)
	if note == "" {
		return nil
	}

	plan := &core.ModelAcquisitionPlan{
		SchemaVersion:  "v1",
		CanMaterialize: true,
		DefaultChoice:  note,
		Strategy:       "user_clarified_strategy",
		Confidence:     "user_clarified",
		Evidence: []core.ModelAcquisitionPlanEvidence{
			{
				Detail:  "user clarified model source",
				Snippet: note,
			},
		},
	}
	if expectedOutputPath := normalizeModelPathValue(selectedModelPath); expectedOutputPath != "" {
		plan.ExpectedOutputPath = expectedOutputPath
	}
	if clarification.RuntimeChangePolicy != "" {
		plan.Evidence = append(plan.Evidence, core.ModelAcquisitionPlanEvidence{
			Detail:  "runtime change policy",
			Snippet: string(clarification.RuntimeChangePolicy),
		})
	}
	return plan
}

func cloneModelAcquisitionPlan(plan *core.ModelAcquisitionPlan) *core.ModelAcquisitionPlan {
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
