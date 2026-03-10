package inspect

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

var pythonVersionPattern = regexp.MustCompile(`(?i)python\s+(\d+)\.(\d+)`)

func inspectRuntimeContract(snapshot core.WorkspaceSnapshot, status *core.IntegrationStatus) {
	runtimeState := snapshot.Runtime
	if !runtimeState.ProbeRan {
		return
	}

	if !runtimeState.PyProjectPresent {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeRuntimeProjectUnsupported,
			Message:  "Concierge v1 can only run local Python checks in Poetry-managed projects, and this repo does not have a pyproject.toml file",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if !runtimeState.SupportedProject {
		message := strings.TrimSpace(runtimeState.ProjectSupportReason)
		if message == "" {
			message = "pyproject.toml does not declare a Poetry-managed project"
		}
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeRuntimeProjectUnsupported,
			Message:  fmt.Sprintf("Concierge v1 can only run local Python checks in Poetry-managed projects: %s", message),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if !runtimeState.PoetryFound {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePoetryNotFound,
			Message:  "Poetry is required for Concierge local runtime checks, but `poetry` was not found in PATH",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if snapshot.RuntimeProfile == nil || strings.TrimSpace(snapshot.RuntimeProfile.InterpreterPath) == "" {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePoetryEnvironmentUnresolved,
			Message:  "I could not resolve an existing Poetry environment for this project yet, so local Python checks are blocked",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}
	if !snapshot.RuntimeProfile.DependenciesReady {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePoetryCheckFailed,
			Message:  "the Poetry project metadata or lock state is not healthy yet; run the runtime step before local Python checks",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
	}
	if !snapshot.RuntimeProfile.CodeLoaderReady {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodeCodeLoaderMissing,
			Message:  "the resolved Poetry environment cannot import `code_loader`, so Tensorleap runtime code is not ready yet",
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
	}

	versionString := strings.TrimSpace(snapshot.RuntimeProfile.PythonVersion)
	if versionString == "" {
		versionString = strings.TrimSpace(runtimeState.ResolvedPythonVersion)
	}
	if versionString == "" {
		return
	}

	major, minor, ok := parsePythonMajorMinor(versionString)
	if !ok {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePythonVersionUnsupported,
			Message:  fmt.Sprintf("unable to parse the resolved Poetry Python version from %q", versionString),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if major < 3 || (major == 3 && minor < 8) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePythonVersionUnsupported,
			Message:  fmt.Sprintf("resolved Poetry runtime uses Python %d.%d; Concierge requires Python 3.8+", major, minor),
			Severity: core.SeverityError,
			Scope:    core.IssueScopeEnvironment,
		})
	}
}

func parsePythonMajorMinor(versionString string) (int, int, bool) {
	matches := pythonVersionPattern.FindStringSubmatch(strings.TrimSpace(versionString))
	if len(matches) != 3 {
		return 0, 0, false
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, false
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, false
	}

	return major, minor, true
}
