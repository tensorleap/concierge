package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tensorleap/concierge/internal/core"
	"github.com/tensorleap/concierge/internal/observe"
)

const claudeCommand = "claude"

const (
	defaultTimeout      = 15 * time.Minute
	streamProbeTimeout  = 5 * time.Second
	heartbeatInterval   = 1 * time.Second
	transcriptFileMode  = 0o644
	transcriptFolderMode = 0o755
)

var (
	defaultClaudeArgs       = []string{"--print", "--output-format", "stream-json", "--include-partial-messages", "--verbose", "--permission-mode", "bypassPermissions"}
	defaultClaudeBufferArgs = []string{"--print", "--output-format", "text", "--permission-mode", "bypassPermissions"}
)

// Runner executes one task-scoped command invocation and writes live artifacts.
type Runner struct {
	timeout   time.Duration
	lookPath  func(string) (string, error)
	observer  observe.Sink
	interrupt <-chan struct{}
}

// NewRunner creates an agent runner that invokes Claude Code.
func NewRunner() *Runner {
	return &Runner{
		timeout:  defaultTimeout,
		lookPath: exec.LookPath,
		observer: observe.NopSink{},
	}
}

// SetObserver configures the live event sink used during agent execution.
func (r *Runner) SetObserver(sink observe.Sink) {
	if sink == nil {
		sink = observe.NopSink{}
	}
	r.observer = sink
}

// SetInterruptChannel configures a channel that cancels the current Claude step only.
func (r *Runner) SetInterruptChannel(ch <-chan struct{}) {
	r.interrupt = ch
}

// CheckAvailability reports whether Claude Code can be resolved from PATH.
func (r *Runner) CheckAvailability() error {
	r.ensureDefaults()
	_, err := r.resolveCommand()
	return err
}

// Run executes the task via Claude Code and writes transcript/raw-stream artifacts.
func (r *Runner) Run(ctx context.Context, task AgentTask) (AgentResult, error) {
	r.ensureDefaults()

	if strings.TrimSpace(task.Objective) == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_objective", "agent task objective is required")
	}
	repoRoot := strings.TrimSpace(task.RepoRoot)
	if repoRoot == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_repo_root", "agent task repo root is required")
	}
	transcriptPath := strings.TrimSpace(task.TranscriptPath)
	if transcriptPath == "" {
		return AgentResult{}, core.NewError(core.KindUnknown, "agent.runner.task_transcript_path", "agent task transcript path is required")
	}
	if err := validateTaskContextPayload(task); err != nil {
		return AgentResult{}, err
	}

	commandPath, err := r.resolveCommand()
	if err != nil {
		return AgentResult{}, err
	}

	runCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok && r.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	systemPrompt := BuildClaudeSystemPrompt()
	taskPrompt := BuildClaudeTaskPrompt(task)
	args := append(append([]string(nil), defaultClaudeArgs...), "--system-prompt", systemPrompt, taskPrompt)
	rawStreamPath := filepath.Join(filepath.Dir(transcriptPath), "agent.stream.jsonl")

	if !r.streamingSupported(runCtx, commandPath) {
		r.emit(observe.Event{Kind: observe.EventFallback, Message: "Claude CLI does not support stream-json; falling back to buffered execution"})
		return r.runBuffered(runCtx, repoRoot, commandPath, systemPrompt, taskPrompt, transcriptPath)
	}

	return r.runStreaming(runCtx, repoRoot, commandPath, args, systemPrompt, taskPrompt, transcriptPath, rawStreamPath)
}

func (r *Runner) ensureDefaults() {
	if r.lookPath == nil {
		r.lookPath = exec.LookPath
	}
	if r.observer == nil {
		r.observer = observe.NopSink{}
	}
	if r.timeout <= 0 {
		r.timeout = defaultTimeout
	}
}

func (r *Runner) resolveCommand() (string, error) {
	path, err := r.lookPath(claudeCommand)
	if err != nil {
		return "", core.WrapError(
			core.KindMissingDependency,
			"agent.runner.command_lookup",
			fmt.Errorf("Claude CLI is not available on PATH (expected %q): %w", claudeCommand, err),
		)
	}
	return path, nil
}

func (r *Runner) streamingSupported(ctx context.Context, commandPath string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, streamProbeTimeout)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, commandPath, "--help").CombinedOutput()
	if err != nil {
		return false
	}
	text := string(output)
	return strings.Contains(text, "--output-format") &&
		strings.Contains(text, "stream-json") &&
		strings.Contains(text, "--include-partial-messages")
}

func (r *Runner) runBuffered(
	ctx context.Context,
	repoRoot, commandPath, systemPrompt, taskPrompt, transcriptPath string,
) (AgentResult, error) {
	args := append(append([]string(nil), defaultClaudeBufferArgs...), "--system-prompt", systemPrompt, taskPrompt)
	stdout, stderr, runErr := runAgentCommand(ctx, repoRoot, commandPath, args)
	if writeErr := writeTranscript(transcriptPath, commandPath, args, systemPrompt, taskPrompt, stdout, stderr, runErr); writeErr != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.transcript_write", writeErr)
	}
	if runErr != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.run", runErr)
	}
	now := time.Now().UTC()
	return AgentResult{
		Applied:        true,
		TranscriptPath: transcriptPath,
		Summary:        "agent task completed",
		LastActivityAt: now,
		Evidence: []core.EvidenceItem{
			{Name: "agent.command", Value: commandPath},
		},
	}, nil
}

func (r *Runner) runStreaming(
	ctx context.Context,
	repoRoot, commandPath string,
	args []string,
	systemPrompt, taskPrompt, transcriptPath, rawStreamPath string,
) (AgentResult, error) {
	if err := os.MkdirAll(filepath.Dir(transcriptPath), transcriptFolderMode); err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.transcript_dir", err)
	}

	transcript, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, transcriptFileMode)
	if err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.transcript_open", err)
	}
	defer transcript.Close()

	rawStream, err := os.OpenFile(rawStreamPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, transcriptFileMode)
	if err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.raw_stream_open", err)
	}
	defer rawStream.Close()

	if _, err := io.WriteString(transcript, buildTranscriptHeader(commandPath, args, systemPrompt, taskPrompt)); err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.transcript_header", err)
	}

	cmd := exec.CommandContext(ctx, commandPath, args...)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.stdout_pipe", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.stderr_pipe", err)
	}

	if err := cmd.Start(); err != nil {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.start", err)
	}

	r.emit(observe.Event{Kind: observe.EventAgentStarted, Message: "Claude started", Data: map[string]string{"command": commandPath}})

	var interrupted atomic.Bool
	if r.interrupt != nil {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-r.interrupt:
				interrupted.Store(true)
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
			}
		}()
	}

	stream := newClaudeStream(transcript, rawStream, r.observer)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stream.consumeStdout(stdout)
	}()
	go func() {
		defer wg.Done()
		stream.consumeStderr(stderr)
	}()

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				label := "Claude running"
				if current := strings.TrimSpace(stream.currentHeadline()); current != "" {
					label = current
				}
				r.emit(observe.Event{
					Kind:    observe.EventAgentHeartbeat,
					Message: label,
					Detail:  stream.lastActivityAt().Format(time.RFC3339),
				})
			}
		}
	}()

	waitErr := cmd.Wait()
	close(done)
	wg.Wait()
	stream.finish(waitErr)

	if waitErr != nil && !interrupted.Load() {
		return AgentResult{}, core.WrapError(core.KindUnknown, "agent.runner.run", waitErr)
	}

	lastActivity := stream.lastActivityAt()
	if interrupted.Load() {
		r.emit(observe.Event{Kind: observe.EventAgentInterrupted, Message: "Claude was interrupted for this step"})
		return AgentResult{
			Applied:        true,
			TranscriptPath: transcriptPath,
			RawStreamPath:  rawStreamPath,
			Summary:        "agent task interrupted",
			Interrupted:    true,
			LastActivityAt: lastActivity,
			Evidence: []core.EvidenceItem{
				{Name: "agent.command", Value: commandPath},
			},
		}, nil
	}

	r.emit(observe.Event{Kind: observe.EventAgentFinished, Message: "Claude finished the step"})
	return AgentResult{
		Applied:        true,
		TranscriptPath: transcriptPath,
		RawStreamPath:  rawStreamPath,
		Summary:        "agent task completed",
		LastActivityAt: lastActivity,
		Evidence: []core.EvidenceItem{
			{Name: "agent.command", Value: commandPath},
		},
	}, nil
}

func (r *Runner) emit(event observe.Event) {
	if r == nil || r.observer == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	r.observer.Emit(event)
}

func runAgentCommand(ctx context.Context, dir, command string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stderrText := strings.TrimSpace(stderr.String()); stderrText != "" {
			err = fmt.Errorf("%w (stderr: %s)", err, stderrText)
		}
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

func validateTaskContextPayload(task AgentTask) error {
	if task.ScopePolicy == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_scope_policy", "agent task scope policy is required")
	}
	if task.RepoContext == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_repo_context", "agent task repository context is required")
	}
	if strings.TrimSpace(task.RepoContext.RepoRoot) == "" {
		return core.NewError(core.KindUnknown, "agent.runner.task_repo_context", "agent task repository context repoRoot is required")
	}
	if task.DomainKnowledge == nil {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge slice is required")
	}
	if strings.TrimSpace(task.DomainKnowledge.Version) == "" {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge version is required")
	}

	sectionIDs := normalizedUniqueOrdered(task.DomainKnowledge.SectionIDs)
	if len(sectionIDs) == 0 {
		return core.NewError(core.KindUnknown, "agent.runner.task_domain_knowledge", "agent task domain knowledge section IDs are required")
	}

	missingSections := make([]string, 0)
	for _, sectionID := range sectionIDs {
		if strings.TrimSpace(task.DomainKnowledge.Sections[sectionID]) != "" {
			continue
		}
		missingSections = append(missingSections, sectionID)
	}
	if len(missingSections) > 0 {
		return core.NewError(
			core.KindUnknown,
			"agent.runner.task_domain_knowledge",
			fmt.Sprintf("agent task domain knowledge is missing section body for: %s", strings.Join(missingSections, ", ")),
		)
	}

	return nil
}

func buildTranscriptHeader(command string, args []string, systemPrompt, taskPrompt string) string {
	var b strings.Builder
	b.WriteString("System prompt:\n")
	b.WriteString(strings.TrimSpace(systemPrompt))
	b.WriteString("\n\n")

	b.WriteString("Task prompt:\n")
	b.WriteString(strings.TrimSpace(taskPrompt))
	b.WriteString("\n\n")

	b.WriteString("Command:\n")
	b.WriteString(command)
	if len(args) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(args, " "))
	}
	b.WriteString("\n\nStream:\n")
	return b.String()
}

func writeTranscript(path, command string, args []string, systemPrompt, taskPrompt string, stdout, stderr []byte, runErr error) error {
	if err := os.MkdirAll(filepath.Dir(path), transcriptFolderMode); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString(buildTranscriptHeader(command, args, systemPrompt, taskPrompt))
	stdoutText := strings.TrimSpace(string(stdout))
	if stdoutText == "" {
		stdoutText = "<empty>"
	}
	b.WriteString(stdoutText)
	b.WriteString("\n\n")
	if runErr != nil {
		b.WriteString("Run error:\n")
		b.WriteString(runErr.Error())
		b.WriteString("\n\n")
	}
	b.WriteString("STDERR:\n")
	stderrText := strings.TrimSpace(string(stderr))
	if stderrText == "" {
		stderrText = "<empty>"
	}
	b.WriteString(stderrText)
	b.WriteString("\n")

	return os.WriteFile(path, []byte(b.String()), transcriptFileMode)
}

type claudeStream struct {
	transcript *os.File
	raw        *os.File
	observer   observe.Sink

	mu             sync.Mutex
	lastActivity   time.Time
	currentTool    string
	currentDetail  string
	currentMessage strings.Builder
}

func newClaudeStream(transcript, raw *os.File, observer observe.Sink) *claudeStream {
	return &claudeStream{
		transcript:    transcript,
		raw:           raw,
		observer:      observer,
		lastActivity:  time.Now().UTC(),
		currentDetail: "",
	}
}

func (s *claudeStream) consumeStdout(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.recordActivity()
		s.writeRaw(line)
		s.handleJSONLine(line)
	}
}

func (s *claudeStream) consumeStderr(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.recordActivity()
		s.writeTranscriptLine("[stderr] " + line)
		s.emit(observe.Event{Kind: observe.EventAgentStderr, Detail: line})
	}
}

func (s *claudeStream) handleJSONLine(line string) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		s.writeTranscriptLine(line)
		return
	}

	switch strings.TrimSpace(stringValue(payload["type"])) {
	case "stream_event":
		s.handleStreamEvent(payload)
	case "assistant":
		s.handleAssistant(payload)
	case "result":
		s.writeTranscriptLine("[result] " + line)
	}
}

func (s *claudeStream) handleStreamEvent(payload map[string]any) {
	event, _ := payload["event"].(map[string]any)
	switch strings.TrimSpace(stringValue(event["type"])) {
	case "content_block_start":
		block, _ := event["content_block"].(map[string]any)
		if stringValue(block["type"]) != "tool_use" {
			return
		}
		s.mu.Lock()
		s.currentTool = stringValue(block["name"])
		s.currentDetail = extractToolDetail(block)
		message := observeToolMessage(s.currentTool, s.currentDetail)
		s.mu.Unlock()
		s.writeTranscriptLine("[tool] " + message)
		s.emit(observe.Event{Kind: observe.EventAgentTool, Message: message, Detail: s.currentDetail, Data: map[string]string{"tool": s.currentTool}})
	case "content_block_delta":
		delta, _ := event["delta"].(map[string]any)
		switch stringValue(delta["type"]) {
		case "input_json_delta":
			partial := stringValue(delta["partial_json"])
			s.mu.Lock()
			s.currentDetail = mergeToolDetail(s.currentDetail, partial)
			toolName := s.currentTool
			detail := s.currentDetail
			message := observeToolMessage(toolName, detail)
			s.mu.Unlock()
			s.emit(observe.Event{Kind: observe.EventAgentTool, Message: message, Detail: detail, Data: map[string]string{"tool": toolName}})
		case "text_delta":
			text := strings.TrimSpace(stringValue(delta["text"]))
			if text == "" {
				return
			}
			s.writeTranscriptLine(text)
			s.emit(observe.Event{Kind: observe.EventAgentMessage, Message: "Reasoning about the fix", Detail: text})
		}
	case "content_block_stop":
		s.mu.Lock()
		s.currentTool = ""
		s.currentDetail = ""
		s.mu.Unlock()
	}
}

func (s *claudeStream) handleAssistant(payload map[string]any) {
	message, _ := payload["message"].(map[string]any)
	content, _ := message["content"].([]any)
	for _, entry := range content {
		item, _ := entry.(map[string]any)
		switch stringValue(item["type"]) {
		case "tool_use":
			toolName := stringValue(item["name"])
			detail := extractToolDetail(item)
			msg := observeToolMessage(toolName, detail)
			s.writeTranscriptLine("[tool] " + msg)
			s.emit(observe.Event{Kind: observe.EventAgentTool, Message: msg, Detail: detail, Data: map[string]string{"tool": toolName}})
		case "text":
			text := strings.TrimSpace(stringValue(item["text"]))
			if text == "" {
				continue
			}
			s.writeTranscriptLine(text)
			s.emit(observe.Event{Kind: observe.EventAgentMessage, Message: "Reasoning about the fix", Detail: text})
		}
	}
}

func (s *claudeStream) finish(waitErr error) {
	if waitErr != nil {
		s.writeTranscriptLine("Run error: " + waitErr.Error())
	}
}

func (s *claudeStream) recordActivity() {
	s.mu.Lock()
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *claudeStream) lastActivityAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivity
}

func (s *claudeStream) currentHeadline() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return observeToolMessage(s.currentTool, s.currentDetail)
}

func (s *claudeStream) writeRaw(line string) {
	if s.raw == nil {
		return
	}
	_, _ = io.WriteString(s.raw, strings.TrimRight(line, "\n")+"\n")
}

func (s *claudeStream) writeTranscriptLine(line string) {
	if s.transcript == nil {
		return
	}
	_, _ = io.WriteString(s.transcript, strings.TrimRight(line, "\n")+"\n")
}

func (s *claudeStream) emit(event observe.Event) {
	if s.observer == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	s.observer.Emit(event)
}

func observeToolMessage(toolName, detail string) string {
	return observeToolHeadline(toolName, detail)
}

func observeToolHeadline(toolName, detail string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	detail = strings.TrimSpace(detail)
	switch name {
	case "read", "grep", "glob", "ls":
		if detail != "" {
			return fmt.Sprintf("Scanning repository code: %s", detail)
		}
		return "Scanning repository code"
	case "bash":
		if detail != "" {
			return fmt.Sprintf("Running repo check: %s", detail)
		}
		return "Running a repo check"
	case "edit", "multiedit", "write", "notebookedit":
		if detail != "" {
			return fmt.Sprintf("Editing %s", detail)
		}
		return "Editing repository files"
	default:
		if toolName != "" {
			return fmt.Sprintf("Claude is using %s", toolName)
		}
		return "Claude running"
	}
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", value)
	}
}

func extractToolDetail(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if input, ok := payload["input"].(map[string]any); ok {
		for _, key := range []string{"file_path", "path", "command", "pattern", "query"} {
			if value := stringValue(input[key]); strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	for _, key := range []string{"file_path", "path", "command", "pattern", "query"} {
		if value := stringValue(payload[key]); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mergeToolDetail(current, partial string) string {
	merged := strings.TrimSpace(current + partial)
	for _, key := range []string{`"file_path":"`, `"path":"`, `"command":"`, `"pattern":"`, `"query":"`} {
		index := strings.Index(merged, key)
		if index < 0 {
			continue
		}
		value := merged[index+len(key):]
		if end := strings.Index(value, `"`); end >= 0 {
			return value[:end]
		}
	}
	return strings.TrimSpace(current)
}
