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

	if !runtimeState.PythonFound {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePythonNotFound,
			Message:  "python runtime was not found in PATH",
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if strings.TrimSpace(runtimeState.PythonVersion) == "" {
		return
	}

	major, minor, ok := parsePythonMajorMinor(runtimeState.PythonVersion)
	if !ok {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePythonVersionUnsupported,
			Message:  fmt.Sprintf("unable to parse python version from %q", runtimeState.PythonVersion),
			Severity: core.SeverityWarning,
			Scope:    core.IssueScopeEnvironment,
		})
		return
	}

	if major < 3 || (major == 3 && minor < 8) {
		status.Issues = append(status.Issues, core.Issue{
			Code:     core.IssueCodePythonVersionUnsupported,
			Message:  fmt.Sprintf("python %d.%d is unsupported; python 3.8+ is recommended", major, minor),
			Severity: core.SeverityWarning,
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
