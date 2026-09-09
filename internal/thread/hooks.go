package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeHookEvent is the stable subset of Claude Code hook payloads Thread
// needs. Unknown fields are intentionally ignored for forward compatibility.
type ClaudeHookEvent struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	HookEventName        string `json:"hook_event_name"`
	Reason               string `json:"reason"`
	Prompt               string `json:"prompt"`
	LastAssistantMessage string `json:"last_assistant_message"`
	TranscriptPath       string `json:"transcript_path"`
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
	project, err := s.ResolveProject("", event.Cwd)
	if err != nil {
		return "", err
	}
	name := "Claude session " + event.SessionID
	n := NewNote("session", name)
	n.ID = "session-" + Hash([]byte(event.SessionID + "\x00" + project.ID))[:40]
	n.Source = "claude-hook"
	n.Status = "active"
	n.Repo = event.Cwd
	n.Project = s.Link(project)
	n.Extra = map[string]any{"session_id": event.SessionID, "cwd": event.Cwd, "hook_event": event.HookEventName}
	if event.TranscriptPath != "" {
		n.Extra["transcript_path"] = event.TranscriptPath
	}
	if event.Reason != "" {
		n.Extra["reason"] = event.Reason
	}

	// SessionStart is idempotent. Other events update the same session note.
	if existing, findErr := s.Find(n.ID); findErr == nil {
		return existing.Path, s.Update(n.ID, func(current *Note) error {
			current.Extra["last_hook_event"] = event.HookEventName
			if event.Reason != "" {
				current.Extra["reason"] = event.Reason
			}
			if event.LastAssistantMessage != "" {
				current.Next = firstLine(event.LastAssistantMessage)
				current.Body += "\n## " + event.HookEventName + "\n\n" + bounded(event.LastAssistantMessage) + "\n"
			}
			if event.HookEventName == "SessionEnd" {
				current.Status = "paused"
			}
			return nil
		})
	} else if !os.IsNotExist(findErr) && !strings.Contains(findErr.Error(), "not found") {
		return "", findErr
	}
	n.Body = "\nStarted in `" + event.Cwd + "`.\n\n"
	if event.LastAssistantMessage != "" {
		n.Body += "## " + event.HookEventName + "\n\n" + bounded(event.LastAssistantMessage) + "\n"
	}
	return s.New(n)
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
