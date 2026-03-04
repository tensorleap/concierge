package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func promptProjectRootSelection(in io.Reader, out io.Writer, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no project root candidates available")
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	if out == nil {
		out = io.Discard
	}
	if _, err := fmt.Fprintln(out, "Project Selection"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(out, "Choose where Concierge should run:"); err != nil {
		return "", err
	}
	for i, candidate := range candidates {
		if _, err := fmt.Fprintf(out, "  %d. %s\n", i+1, candidate); err != nil {
			return "", err
		}
	}
	if _, err := fmt.Fprint(out, "Selection [1-", len(candidates), "]: "); err != nil {
		return "", err
	}

	line, err := readPromptLine(in)
	if err != nil {
		return "", err
	}

	selected, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || selected < 1 || selected > len(candidates) {
		return "", fmt.Errorf("invalid project root selection %q", strings.TrimSpace(line))
	}

	return candidates[selected-1], nil
}

func promptModelCandidateSelection(in io.Reader, out io.Writer, candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no model candidates available")
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	if out == nil {
		out = io.Discard
	}
	if _, err := fmt.Fprintln(out, "Model Selection"); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintln(out, "Multiple model files were found. Choose one for @tensorleap_load_model:"); err != nil {
		return "", err
	}
	for i, candidate := range candidates {
		if _, err := fmt.Fprintf(out, "  %d. %s\n", i+1, candidate); err != nil {
			return "", err
		}
	}
	if _, err := fmt.Fprint(out, "Selection [1-", len(candidates), "]: "); err != nil {
		return "", err
	}

	line, err := readPromptLine(in)
	if err != nil {
		return "", err
	}

	selected, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || selected < 1 || selected > len(candidates) {
		return "", fmt.Errorf("invalid model selection %q", strings.TrimSpace(line))
	}

	return candidates[selected-1], nil
}

func promptApproval(in io.Reader, out io.Writer, message string, enableColor bool) (bool, error) {
	if out == nil {
		out = io.Discard
	}
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		trimmedMessage = "I have a suggested next step."
	}
	if _, err := fmt.Fprintln(out, paint("I >", ansiBold+ansiCyan, enableColor)); err != nil {
		return false, err
	}
	for _, line := range strings.Split(trimmedMessage, "\n") {
		if _, err := fmt.Fprintf(out, "  %s\n", line); err != nil {
			return false, err
		}
	}

	promptText := fmt.Sprintf("%s Continue now? [y/N]:", paint("You >", ansiBold+ansiBlue, enableColor))
	return promptYesNo(in, out, promptText, false)
}

func promptYesNo(in io.Reader, out io.Writer, prompt string, defaultYes bool) (bool, error) {
	if out == nil {
		out = io.Discard
	}

	promptText := strings.TrimSpace(prompt)
	if promptText == "" {
		if defaultYes {
			promptText = "Proceed? [Y/n]:"
		} else {
			promptText = "Proceed? [y/N]:"
		}
	}
	if _, err := fmt.Fprintf(out, "%s ", promptText); err != nil {
		return false, err
	}

	line, err := readPromptLine(in)
	if err != nil {
		return false, err
	}

	normalized := strings.ToLower(strings.TrimSpace(line))
	switch normalized {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	case "":
		return defaultYes, nil
	default:
		return false, fmt.Errorf("invalid confirmation response %q", line)
	}
}

func readPromptLine(in io.Reader) (string, error) {
	if in == nil {
		return "", io.EOF
	}
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}
