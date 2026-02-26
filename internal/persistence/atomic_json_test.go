package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type atomicPayload struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestWriteJSONAtomicCreatesFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".concierge", "reports", "snapshot-1.json")
	payload := atomicPayload{Name: "first", Value: 1}

	if err := WriteJSONAtomic(target, payload); err != nil {
		t.Fatalf("WriteJSONAtomic returned error: %v", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded atomicPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded != payload {
		t.Fatalf("expected payload %+v, got %+v", payload, decoded)
	}
}

func TestWriteJSONAtomicOverwritesSafely(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".concierge", "reports", "snapshot-2.json")
	first := atomicPayload{Name: "first", Value: 1}
	second := atomicPayload{Name: "second", Value: 2}

	if err := WriteJSONAtomic(target, first); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	if err := WriteJSONAtomic(target, second); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded atomicPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded != second {
		t.Fatalf("expected payload %+v, got %+v", second, decoded)
	}
}

func TestWriteJSONAtomicRejectsInvalidPath(t *testing.T) {
	if err := WriteJSONAtomic("", atomicPayload{Name: "invalid"}); err == nil {
		t.Fatal("expected error for empty path")
	}
}
