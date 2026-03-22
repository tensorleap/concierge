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

// ParseHarnessResult captures both valid NDJSON events and skipped non-JSON lines.
type ParseHarnessResult struct {
	Events []HarnessEvent
	Noise  []string // non-JSON lines that were skipped
}

// ParseHarnessEvents parses NDJSON harness output, skipping non-JSON lines.
// Non-JSON lines are collected as Noise for diagnostics.
// An error is returned only when the output contains noise but zero valid events.
func ParseHarnessEvents(raw []byte) (ParseHarnessResult, error) {
	var events []HarnessEvent
	var noise []string

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event HarnessEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			noise = append(noise, line)
			continue
		}

		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return ParseHarnessResult{}, fmt.Errorf("validate.harness.scan: %w", err)
	}

	if len(events) == 0 && len(noise) > 0 {
		return ParseHarnessResult{Noise: noise}, fmt.Errorf("validate.harness.parse: no valid NDJSON events found; %d non-JSON lines skipped", len(noise))
	}

	return ParseHarnessResult{Events: events, Noise: noise}, nil
}

func messageOrDefault(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}
