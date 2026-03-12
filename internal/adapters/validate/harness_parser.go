package validate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// HarnessEvent is one NDJSON event emitted by the harness process.
type HarnessEvent struct {
	Event         string `json:"event"`
	Status        string `json:"status,omitempty"`
	Message       string `json:"message,omitempty"`
	Name          string `json:"name,omitempty"`
	Symbol        string `json:"symbol,omitempty"`
	HandlerKind   string `json:"handler_kind,omitempty"`
	Subset        string `json:"subset,omitempty"`
	SampleID      string `json:"sample_id,omitempty"`
	SampleOffset  int    `json:"sample_offset,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	Count         int    `json:"count,omitempty"`
	Shape         []int  `json:"shape,omitempty"`
	DType         string `json:"dtype,omitempty"`
	ExpectedShape []int  `json:"expected_shape,omitempty"`
	ExpectedDType string `json:"expected_dtype,omitempty"`
	Finite        *bool  `json:"finite,omitempty"`
}

// ParseHarnessEvents parses NDJSON harness output.
func ParseHarnessEvents(raw []byte) ([]HarnessEvent, error) {
	events := make([]HarnessEvent, 0)

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event HarnessEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("validate.harness.parse: line %d: %w", lineNo, err)
		}

		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("validate.harness.scan: %w", err)
	}

	return events, nil
}

func messageOrDefault(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}
