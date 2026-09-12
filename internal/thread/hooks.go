package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var hookDeadline = 3 * time.Second

// RunHookBounded keeps editor lifecycle hooks fail-open. A stalled vault may
// lose this observation, but it cannot stall the client session indefinitely.
func RunHookBounded(capture func() (string, error)) (output, skipped string, err error) {
	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		value, captureErr := capture()
		done <- result{output: value, err: captureErr}
	}()
	timer := time.NewTimer(hookDeadline)
	defer timer.Stop()
	select {
	case got := <-done:
		if IsVaultUnavailable(got.err) {
			return "", got.err.Error(), nil
		}
		return got.output, "", got.err
	case <-timer.C:
		return "", fmt.Sprintf("vault operation exceeded %s", hookDeadline), nil
	}
}

// ClaudeHookEvent is the stable subset of Claude Code hook payloads Thread
// needs. Unknown fields are intentionally ignored for forward compatibility.
type ClaudeHookEvent struct {
	Provider             string `json:"provider"`
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	HookEventName        string `json:"hook_event_name"`
	Reason               string `json:"reason"`
	Prompt               string `json:"prompt"`
	LastAssistantMessage string `json:"last_assistant_message"`
	TranscriptPath       string `json:"transcript_path"`
	EventID              string `json:"event_id"`
	OccurredAt           string `json:"occurred_at"`
}

func DecodeHook(data []byte) (ClaudeHookEvent, error) {
	var event ClaudeHookEvent
	if len(data) > 4*1024*1024 {
		return event, errors.New("hook payload exceeds 4 MiB")
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return event, fmt.Errorf("invalid hook JSON: %w", err)
	}
	if event.SessionID == "" || event.Cwd == "" {
		return event, errors.New("hook payload requires session_id and cwd")
	}
	return event, nil
}

func (s *Store) HandleClaudeHook(event ClaudeHookEvent) (string, error) {
	types := map[string]string{
		"SessionStart": "session.started", "session-start": "session.started",
		"SessionEnd": "session.stopped", "session-end": "session.stopped",
		"Stop": "session.idle", "stop": "session.idle",
		"UserPromptSubmit": "prompt.started", "prompt-started": "prompt.started",
		"PreToolUse": "tool.before", "PostToolUse": "tool.after",
	}
	eventType, ok := types[event.HookEventName]
	if !ok {
		return "", fmt.Errorf("unsupported Claude hook event %q", event.HookEventName)
	}
	if event.Provider != "" && event.Provider != "claude" {
		return "", errors.New("Claude hook adapter accepts only Claude events; use the normalized event interface")
	}
	project, err := s.ResolveProject("", event.Cwd)
	if err != nil {
		return "", err
	}
	e := Event{Schema: 1, ID: event.EventID, Type: eventType, Source: "claude",
		SessionID: event.SessionID, ProjectID: project.ID, Repo: event.Cwd,
		Machine: Machine(), OccurredAt: event.OccurredAt, Text: event.LastAssistantMessage,
		Data: map[string]any{"hook_event": event.HookEventName}}
	if event.Prompt != "" {
		e.Data["prompt"] = event.Prompt
	}
	if event.TranscriptPath != "" {
		e.References = []string{event.TranscriptPath}
	}
	if event.Reason != "" {
		e.Data["reason"] = event.Reason
	}
	if e.OccurredAt == "" {
		e.Data["timestamp_basis"] = "received; producer did not supply occurred_at"
	}
	if e.ID == "" {
		// Hooks without delivery IDs cannot distinguish identical occurrences.
		// Deduplicate exact payloads rather than inventing extra lifecycle facts.
		raw, err := json.Marshal(e)
		if err != nil {
			return "", err
		}
		e.ID = "hook-" + Hash(raw)
	}
	return s.captureEvent(e, e.OccurredAt == "")
}

func firstLine(text string) string {
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if len([]rune(line)) > 180 {
		return string([]rune(line)[:180]) + "…"
	}
	return line
}
func bounded(text string) string {
	if len([]byte(text)) > 64*1024 {
		return string([]byte(text)[:64*1024]) + "\n[truncated]"
	}
	return strings.TrimSpace(text)
}

func HookConfigExample(binary string) string {
	return fmt.Sprintf(`{
  "SessionStart": [{"hooks":[{"type":"command","command":"%s hook --provider claude --event session-start"}]}],
  "Stop": [{"hooks":[{"type":"command","command":"%s hook --provider claude --event stop"}]}],
  "SessionEnd": [{"hooks":[{"type":"command","command":"%s hook --provider claude --event session-end"}] }]
}`, filepath.Clean(binary), filepath.Clean(binary), filepath.Clean(binary))
}
