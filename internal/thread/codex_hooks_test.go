package thread

import (
	"encoding/json"
	"testing"
)

func TestCodexNativeHooksCaptureWithoutRepository(t *testing.T) {
	s := testStore(t)
	data := map[string]any{"session_id": "native-session", "cwd": "/missing/offline/repo", "turn_id": "turn-1", "hook_event_name": "UserPromptSubmit", "prompt": "Keep this objective", "transcript_path": nil}
	raw, _ := json.Marshal(data)
	for i := 0; i < 2; i++ {
		out, err := s.HandleCodexHook(raw, "UserPromptSubmit")
		if err != nil || out != "" {
			t.Fatalf("capture: %q %v", out, err)
		}
	}
	data["turn_id"] = "turn-2"
	raw, _ = json.Marshal(data)
	if _, err := s.HandleCodexHook(raw, "UserPromptSubmit"); err != nil {
		t.Fatal(err)
	}
	notes, err := s.Notes()
	if err != nil || len(notes) != 2 {
		t.Fatalf("distinct turns/retries: %d %v", len(notes), err)
	}
	for _, n := range notes {
		e, err := noteEvent(n)
		if err != nil || e.Text != "Keep this objective" || n.Status != "" || e.Source != "codex" {
			t.Fatalf("bad observation: %+v %v", e, err)
		}
	}
	if _, err := s.HandleCodexHook(raw, "Stop"); err == nil {
		t.Fatal("accepted mismatched event")
	}
}

func TestCodexNativeHookBoundaries(t *testing.T) {
	s := testStore(t)
	for _, name := range []string{"SessionStart", "SessionEnd", "Stop", "Interrupt", "PreToolUse", "PostToolUse", "PreCompact", "PostCompact"} {
		raw, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": "/missing/offline/repo", "hook_event_name": name, "tool_use_id": "t1", "tool_input": map[string]any{"command": "saved evidence"}})
		if _, err := s.HandleCodexHook(raw, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
