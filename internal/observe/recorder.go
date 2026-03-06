package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tensorleap/concierge/internal/persistence"
)

// Recorder persists live events as append-only JSONL once the snapshot ID is known.
type Recorder struct {
	paths      *persistence.Paths
	mu         sync.Mutex
	snapshotID string
	file       *os.File
	buffer     []Event
}

// NewRecorder creates a new event recorder.
func NewRecorder(projectRoot string) (*Recorder, error) {
	paths, err := persistence.NewPaths(projectRoot)
	if err != nil {
		return nil, err
	}
	return &Recorder{paths: paths}, nil
}

// Emit implements Sink.
func (r *Recorder) Emit(event Event) {
	if r == nil || r.paths == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if strings.TrimSpace(event.SnapshotID) != "" && r.snapshotID == "" {
		r.snapshotID = event.SnapshotID
		if err := r.openLocked(); err == nil {
			for _, buffered := range r.buffer {
				_ = r.writeLocked(buffered)
			}
			r.buffer = nil
		}
	}
	if r.file == nil {
		r.buffer = append(r.buffer, event)
		return
	}
	_ = r.writeLocked(event)
}

func (r *Recorder) openLocked() error {
	if r.file != nil {
		return nil
	}
	path := filepath.Join(r.paths.EvidenceDir(r.snapshotID), "events.jsonl")
	if err := os.MkdirAll(r.paths.EvidenceDir(r.snapshotID), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	r.file = file
	return nil
}

func (r *Recorder) writeLocked(event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.file.Write(append(payload, '\n'))
	return err
}
