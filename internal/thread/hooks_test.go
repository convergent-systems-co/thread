package thread

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClaudeHookSessionIsIdempotentAndLeavesBoundedResumeContext(t *testing.T) {
	s := testStore(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	p := NewNote("project", "Hook project")
	p.ID = "hook-project"
	p.Repo = repo
	p.Domain = "home"
	if _, err := s.New(p); err != nil {
		t.Fatal(err)
	}
	start, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": repo, "hook_event_name": "SessionStart", "transcript_path": "/tmp/transcript.jsonl"})
	first, err := s.HandleClaudeHook(mustHook(t, start))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.HandleClaudeHook(mustHook(t, start))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("session start duplicated")
	}
	stop, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": repo, "hook_event_name": "Stop", "last_assistant_message": "Implemented the first slice.\nDetails follow."})
	if _, err = s.HandleClaudeHook(mustHook(t, stop)); err != nil {
		t.Fatal(err)
	}
	n, err := s.Find("session-" + Hash([]byte("s1\x00hook-project"))[:40])
	if err != nil {
		t.Fatal(err)
	}
	if n.Next != "Implemented the first slice." || n.Status != "active" {
		t.Fatalf("bad stop state: %+v", n)
	}
	end, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": repo, "hook_event_name": "SessionEnd", "reason": "user_exit"})
	if _, err = s.HandleClaudeHook(mustHook(t, end)); err != nil {
		t.Fatal(err)
	}
	n, _ = s.Find(n.ID)
	if n.Status != "paused" || n.Extra["reason"] != "user_exit" {
		t.Fatalf("bad end state: %+v", n)
	}
}

func mustHook(t *testing.T, b []byte) ClaudeHookEvent {
	t.Helper()
	e, err := DecodeHook(b)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestHookRejectsOversizedPayload(t *testing.T) {
	b := make([]byte, 4*1024*1024+1)
	if _, err := DecodeHook(b); err == nil {
		t.Fatal("accepted oversized payload")
	}
}

func TestHookConfigExampleUsesAbsoluteBinary(t *testing.T) {
	s := HookConfigExample("/Users/test/.local/bin/thread")
	if !filepath.IsAbs("/Users/test/.local/bin/thread") || len(s) == 0 {
		t.Fatal("bad config")
	}
}
