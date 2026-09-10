package thread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// HandleCodexHook adapts Codex's public hook payload to Thread's event contract.
// Capture precedes optional orientation and never reads the transcript.
func (s *Store) HandleCodexHook(raw []byte, eventName string) (string, error) {
	if len(raw) > MaxEventBytes {
		return "", errors.New("hook payload exceeds 4 MiB")
	}
	var data map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return "", err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return "", errors.New("expected one hook object")
	}
	str := func(key string) string { value, _ := data[key].(string); return value }
	name := str("hook_event_name")
	if name == "" {
		name = eventName
	}
	if name != eventName {
		return "", errors.New("hook event flag and payload disagree")
	}
	types := map[string]string{"SessionStart": "session.started", "SessionEnd": "session.stopped", "Stop": "session.idle", "Interrupt": "session.idle", "UserPromptSubmit": "prompt.started", "PreToolUse": "tool.before", "PostToolUse": "tool.after", "PreCompact": "checkpoint.requested", "PostCompact": "context.changed"}
	kind, ok := types[name]
	if !ok {
		return "", fmt.Errorf("unsupported Codex hook %q", name)
	}
	if str("session_id") == "" || str("cwd") == "" {
		return "", errors.New("hook requires session_id and cwd")
	}
	if name == "SessionStart" && str("source") == "resume" {
		kind = "session.resumed"
	}
	e := Event{Schema: 1, Source: "codex", Type: kind, SessionID: str("session_id"), Repo: str("cwd"), Machine: Machine(), Data: data, Text: str("last_assistant_message")}
	if name == "UserPromptSubmit" {
		e.Text = str("prompt")
	}
	if path := str("transcript_path"); path != "" {
		e.References = []string{path}
	}
	// Public payloads carry turn/tool identities, but no universal delivery ID.
	// Exact payload retries deduplicate; distinct turns remain distinct.
	identity, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	e.ID = "hook-" + Hash(append([]byte(name+":"), identity...))
	// Unregistered/non-Git working directories must not prevent capture.
	project, projectErr := s.ResolveProject("", e.Repo)
	if projectErr == nil {
		e.ProjectID = project.ID
	}
	e.Data["timestamp_basis"] = "received; producer did not supply occurred_at"
	if _, err := s.captureEvent(e, true); err != nil {
		return "", err
	}
	if name == "SessionStart" && projectErr == nil {
		o, err := s.Orient(project.ID, 3)
		if err != nil {
			return "", fmt.Errorf("event saved; orientation failed: %w", err)
		}
		return RenderOrientation(o), nil
	}
	return "", nil
}
