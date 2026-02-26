package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tensorleap/concierge/internal/core"
)

var nowUnixNano = func() int64 {
	return time.Now().UnixNano()
}

// WriteJSONAtomic writes JSON to path via a same-directory temp file and rename.
func WriteJSONAtomic(path string, v any) error {
	target := strings.TrimSpace(path)
	if target == "" {
		return core.NewError(core.KindUnknown, "persistence.write_json_atomic.path", "target path is required")
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.mkdir", err)
	}

	payload, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.marshal", err)
	}
	payload = append(payload, '\n')

	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", target, os.Getpid(), nowUnixNano())
	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.tmp_open", err)
	}

	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.tmp_write", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.tmp_sync", err)
	}
	if err := tmpFile.Close(); err != nil {
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.tmp_close", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return core.WrapError(core.KindUnknown, "persistence.write_json_atomic.rename", err)
	}

	removeTmp = false
	return nil
}
