package state

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/persistence"
)

// LoadState reads .concierge/state/state.json and returns default state when missing.
func LoadState(projectRoot string) (RunState, error) {
	paths, err := persistence.NewPaths(projectRoot)
	if err != nil {
		return RunState{}, err
	}

	statePath := paths.StateFile()
	raw, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultRunState(paths.ProjectRoot()), nil
		}
		return RunState{}, core.WrapError(core.KindUnknown, "state.load.read", err)
	}

	var state RunState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RunState{}, core.WrapError(core.KindUnknown, "state.load.unmarshal", err)
	}

	if state.Version == 0 {
		state.Version = CurrentVersion
	}
	if state.SelectedProjectRoot == "" {
		state.SelectedProjectRoot = paths.ProjectRoot()
	} else {
		state.SelectedProjectRoot = normalizeRoot(state.SelectedProjectRoot)
	}
	state.RuntimeProfile = cloneRuntimeProfile(state.RuntimeProfile)
	if state.RuntimeProfile != nil {
		state.RuntimeProfile.Fingerprint.ProjectRoot = normalizeRoot(state.RuntimeProfile.Fingerprint.ProjectRoot)
	}
	state.LastBlockingIssues = cloneIssues(state.LastBlockingIssues)

	return state, nil
}

// SaveState writes .concierge/state/state.json atomically.
func SaveState(projectRoot string, state RunState) error {
	paths, err := persistence.NewPaths(projectRoot)
	if err != nil {
		return err
	}

	toWrite := state
	if toWrite.Version == 0 {
		toWrite.Version = CurrentVersion
	}
	if toWrite.SelectedProjectRoot == "" {
		toWrite.SelectedProjectRoot = paths.ProjectRoot()
	} else {
		toWrite.SelectedProjectRoot = normalizeRoot(toWrite.SelectedProjectRoot)
	}
	toWrite.RuntimeProfile = cloneRuntimeProfile(toWrite.RuntimeProfile)
	if toWrite.RuntimeProfile != nil {
		toWrite.RuntimeProfile.Fingerprint.ProjectRoot = normalizeRoot(toWrite.RuntimeProfile.Fingerprint.ProjectRoot)
	}
	toWrite.LastBlockingIssues = cloneIssues(toWrite.LastBlockingIssues)
	toWrite.InvalidationReasons = append([]string(nil), toWrite.InvalidationReasons...)

	if err := persistence.WriteJSONAtomic(paths.StateFile(), toWrite); err != nil {
		return core.WrapError(core.KindUnknown, "state.save.write", err)
	}

	return nil
}
