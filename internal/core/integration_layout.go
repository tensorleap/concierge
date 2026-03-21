package core

const (
	// CanonicalIntegrationEntryFile is the only supported Tensorleap entrypoint filename in Concierge v1.
	CanonicalIntegrationEntryFile = "leap_integration.py"
)

// RequirementsFileCandidates lists standalone requirements files — no companion
// lock file needed. Each is included in leap.yaml if it exists on disk.
var RequirementsFileCandidates = []string{
	"tensorleap_requirements.txt",
	"requirements.txt",
}

// RequirementsFilePairs lists requirements file pairs where both files must
// exist on disk for either to be included (e.g. Poetry needs both).
var RequirementsFilePairs = [][2]string{
	{"pyproject.toml", "poetry.lock"},
}

// AllRequirementsFileCandidates returns a flat list of every requirements
// filename (standalone + both sides of each pair).
func AllRequirementsFileCandidates() []string {
	out := make([]string, len(RequirementsFileCandidates))
	copy(out, RequirementsFileCandidates)
	for _, pair := range RequirementsFilePairs {
		out = append(out, pair[0], pair[1])
	}
	return out
}
