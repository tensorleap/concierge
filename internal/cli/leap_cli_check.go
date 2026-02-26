package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const leapCLILatestReleaseURL = "https://api.github.com/repos/tensorleap/leap-cli/releases/latest"
const leapCLIInstallGuideURL = "https://docs.tensorleap.ai/getting-started/quickstart/quickstart-using-cli"

type leapCLIDiagnostics struct {
	Installed      bool
	CurrentVersion string
	LatestVersion  string
	Status         string
	Note           string
	Action         string
}

var (
	leapCLIPathLookup  = exec.LookPath
	leapCLIRunCommand  = runCommandCombinedOutput
	leapCLIFetchLatest = fetchLatestLeapCLIRelease
	versionPattern     = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)
)

func detectLeapCLIDiagnostics(ctx context.Context) leapCLIDiagnostics {
	_, err := leapCLIPathLookup("leap")
	if err != nil {
		return leapCLIDiagnostics{
			Installed:      false,
			CurrentVersion: "unknown",
			LatestVersion:  "unknown",
			Status:         "missing",
			Note:           "leap CLI was not found in PATH",
			Action:         fmt.Sprintf("Install Leap CLI using the official guide: %s", leapCLIInstallGuideURL),
		}
	}

	currentVersion, currentErr := detectInstalledLeapCLIVersion(ctx)
	if currentErr != nil {
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: "unknown",
			LatestVersion:  "unknown",
			Status:         "unknown",
			Note:           fmt.Sprintf("failed to parse installed leap version: %v", currentErr),
			Action:         "Run `leap version` manually and ensure the command is available in PATH.",
		}
	}

	latestVersion, latestErr := leapCLIFetchLatest(ctx)
	if latestErr != nil {
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: currentVersion,
			LatestVersion:  "unknown",
			Status:         "unknown",
			Note:           fmt.Sprintf("failed to fetch latest leap release: %v", latestErr),
			Action:         fmt.Sprintf("Check network access, then retry. Install/upgrade guidance: %s", leapCLIInstallGuideURL),
		}
	}

	comparison, compareErr := compareSemver(currentVersion, latestVersion)
	if compareErr != nil {
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: currentVersion,
			LatestVersion:  latestVersion,
			Status:         "unknown",
			Note:           fmt.Sprintf("failed to compare leap versions: %v", compareErr),
			Action:         "Verify installed and latest versions manually, then rerun doctor.",
		}
	}

	if comparison < 0 {
		return leapCLIDiagnostics{
			Installed:      true,
			CurrentVersion: currentVersion,
			LatestVersion:  latestVersion,
			Status:         "outdated",
			Note:           "installed leap CLI is older than the latest release",
			Action: fmt.Sprintf(
				"Upgrade Leap CLI to %s using the official guide: %s",
				latestVersion,
				leapCLIInstallGuideURL,
			),
		}
	}

	return leapCLIDiagnostics{
		Installed:      true,
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		Status:         "up_to_date",
		Note:           "installed leap CLI matches the latest release",
		Action:         "No action needed.",
	}
}

func detectInstalledLeapCLIVersion(ctx context.Context) (string, error) {
	output, err := leapCLIRunCommand(ctx, "leap", "version")
	if err != nil {
		output, err = leapCLIRunCommand(ctx, "leap", "--version")
		if err != nil {
			return "", err
		}
	}

	version, found := extractFirstSemver(output)
	if !found {
		return "", fmt.Errorf("no semantic version found in output: %q", output)
	}
	return normalizeSemver(version), nil
}

func fetchLatestLeapCLIRelease(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, leapCLILatestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "concierge/doctor")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from latest release endpoint", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}

	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", fmt.Errorf("latest release payload missing tag_name")
	}
	return normalizeSemver(tag), nil
}

func runCommandCombinedOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)), err
	}
	return strings.TrimSpace(string(output)), nil
}

func extractFirstSemver(value string) (string, bool) {
	token := versionPattern.FindString(strings.TrimSpace(value))
	if token == "" {
		return "", false
	}
	return token, true
}

func normalizeSemver(value string) string {
	version := strings.TrimSpace(value)
	if version == "" {
		return version
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func compareSemver(a, b string) (int, error) {
	av, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	bv, err := parseSemver(b)
	if err != nil {
		return 0, err
	}

	if av.major != bv.major {
		if av.major < bv.major {
			return -1, nil
		}
		return 1, nil
	}
	if av.minor != bv.minor {
		if av.minor < bv.minor {
			return -1, nil
		}
		return 1, nil
	}
	if av.patch != bv.patch {
		if av.patch < bv.patch {
			return -1, nil
		}
		return 1, nil
	}

	if av.prerelease == bv.prerelease {
		return 0, nil
	}
	if av.prerelease == "" {
		return 1, nil
	}
	if bv.prerelease == "" {
		return -1, nil
	}
	if av.prerelease < bv.prerelease {
		return -1, nil
	}
	return 1, nil
}

type semverValue struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

func parseSemver(value string) (semverValue, error) {
	version := strings.TrimPrefix(strings.TrimSpace(value), "v")
	if version == "" {
		return semverValue{}, fmt.Errorf("empty version")
	}

	mainAndBuild := strings.SplitN(version, "+", 2)
	mainAndPre := strings.SplitN(mainAndBuild[0], "-", 2)
	base := mainAndPre[0]
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return semverValue{}, fmt.Errorf("invalid semantic version %q", value)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semverValue{}, fmt.Errorf("invalid major version in %q", value)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semverValue{}, fmt.Errorf("invalid minor version in %q", value)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semverValue{}, fmt.Errorf("invalid patch version in %q", value)
	}

	parsed := semverValue{
		major: major,
		minor: minor,
		patch: patch,
	}
	if len(mainAndPre) == 2 {
		parsed.prerelease = mainAndPre[1]
	}

	return parsed, nil
}
