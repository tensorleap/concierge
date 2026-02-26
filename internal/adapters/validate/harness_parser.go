package validate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tensorleap/concierge/internal/core"
)

// HarnessEvent is one NDJSON event emitted by the harness process.
type HarnessEvent struct {
	Event       string `json:"event"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	Name        string `json:"name,omitempty"`
	Subset      string `json:"subset,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Count       int    `json:"count,omitempty"`
}

// ParseHarnessEvents parses NDJSON harness output and maps known events to issues.
func ParseHarnessEvents(raw []byte) ([]HarnessEvent, []core.Issue, error) {
	events := make([]HarnessEvent, 0)
	issues := make([]core.Issue, 0)

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
			return nil, nil, core.WrapError(
				core.KindUnknown,
				"validate.harness.parse",
				fmt.Errorf("line %d: %w", lineNo, err),
			)
		}

		events = append(events, event)
		if issue, ok := issueForHarnessEvent(event); ok {
			issues = append(issues, issue)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, core.WrapError(core.KindUnknown, "validate.harness.scan", err)
	}

	return events, issues, nil
}

func issueForHarnessEvent(event HarnessEvent) (core.Issue, bool) {
	eventName := strings.ToLower(strings.TrimSpace(event.Event))
	status := strings.ToLower(strings.TrimSpace(event.Status))
	message := strings.TrimSpace(event.Message)

	switch eventName {
	case "preprocess":
		if status == "failed" {
			return harnessIssue(core.IssueCodeHarnessPreprocessFailed, messageOrDefault(message, "harness preprocess failed"), core.IssueScopePreprocess, core.SeverityError), true
		}
		return core.Issue{}, false
	case "encoder_coverage":
		if status == "incomplete" || status == "failed" {
			return harnessIssue(core.IssueCodeHarnessEncoderCoverageIncomplete, messageOrDefault(message, "harness encoder coverage is incomplete"), core.IssueScopeValidation, core.SeverityError), true
		}
		return core.Issue{}, false
	case "validation":
		if status == "failed" {
			return harnessIssue(core.IssueCodeHarnessValidationFailed, messageOrDefault(message, "harness validation failed"), core.IssueScopeValidation, core.SeverityError), true
		}
		return core.Issue{}, false
	case "subset_count", "input_fingerprint", "label_fingerprint":
		return core.Issue{}, false
	default:
		msg := messageOrDefault(message, fmt.Sprintf("unknown harness event %q", strings.TrimSpace(event.Event)))
		return harnessIssue(core.IssueCodeUnknown, msg, core.IssueScopeValidation, core.SeverityInfo), true
	}
}

func harnessIssue(code core.IssueCode, message string, scope core.IssueScope, severity core.Severity) core.Issue {
	return core.Issue{
		Code:     code,
		Message:  message,
		Severity: severity,
		Scope:    scope,
	}
}

func messageOrDefault(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}
