package inspect

import (
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

func inspectLeapCLIContract(snapshot core.WorkspaceSnapshot, status *core.IntegrationStatus) {
	leapState := snapshot.LeapCLI
	if !leapState.ProbeRan {
		return
	}

	if !leapState.Available {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapCLINotFound,
			Message:  "leap CLI was not found in PATH",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeCLI,
		})
		return
	}

	if strings.TrimSpace(leapState.Version) == "" {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapCLIVersionUnavailable,
			Message:  "leap CLI version probe returned no version information",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeCLI,
		})
	}

	if !leapState.Authenticated {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeLeapCLINotAuthenticated,
			Message:  "leap CLI is not authenticated",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeCLI,
		})
	}

	if leapState.ServerInfoReachable {
		return
	}

	message := "leap server info probe failed"
	if strings.TrimSpace(leapState.ServerInfoError) != "" {
		message = fmt.Sprintf("leap server info probe failed: %s", strings.TrimSpace(leapState.ServerInfoError))
	}

	code := core.IssueCodeLeapServerInfoFailed
	lower := strings.ToLower(leapState.ServerInfoError)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "unreachable") || strings.Contains(lower, "timed out") {
		code = core.IssueCodeLeapServerUnreachable
	}

	status.Issues = append(status.Issues, core.Issue{
		Code:     code,
		Message:  message,
		Severity: core.SeverityWarning,
		Scope:    core.IssueScopeServer,
	})
}
