package thread

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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
	o, err := s.Orient(p.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Sessions) != 1 || o.Sessions[0].State != "idle" || len(o.Work) != 0 {
		t.Fatalf("idle observation became work: %+v", o)
	}
	end, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": repo, "hook_event_name": "SessionEnd", "reason": "user_exit"})
	if _, err = s.HandleClaudeHook(mustHook(t, end)); err != nil {
		t.Fatal(err)
	}
	o, err = s.Orient(p.ID, 5)
	if err != nil || len(o.Sessions) != 1 || o.Sessions[0].State != "stopped" {
		t.Fatalf("bad end state: %+v, %v", o, err)
	}
	notes, err := s.Notes()
	if err != nil || len(notes) != 4 {
		t.Fatalf("expected project and three immutable observations: %d, %v", len(notes), err)
	}
}

func TestClaudeHookAutoDiscoversUnknownRepository(t *testing.T) {
	s := testStore(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	p, err := s.HandleClaudeHook(mustHook(t, []byte(`{"session_id":"unknown-session","cwd":"`+repo+`","hook_event_name":"SessionStart"}`)))
	if err != nil {
		t.Fatal(err)
	}
	common := mustCommonDir(t, repo)
	notes, err := s.Notes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected project and event: %+v", notes)
	}
	project, err := s.Project("project-auto-" + Hash([]byte(common))[:32])
	if err != nil || project.Status != "paused" || project.Source != "auto-discovery" {
		t.Fatalf("bad auto-discovered project: %+v, %v", project, err)
	}
	if filepath.Dir(p) != filepath.Join(s.Root, "Thread", ".events") {
		t.Fatalf("event not stored in .events: %s", p)
	}
}

func mustCommonDir(t *testing.T, repo string) string {
	t.Helper()
	b, err := exec.Command("git", "-C", repo, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(b[:len(b)-1])
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

func TestHookVaultFailureDegradesWithoutFailingCaller(t *testing.T) {
	output, skipped, err := RunHookBounded(func() (string, error) {
		return "", &VaultReadError{Path: "Thread/.items/remote.md", Err: errors.New("not downloaded")}
	})
	if err != nil || output != "" || skipped == "" {
		t.Fatalf("hook did not degrade: output=%q skipped=%q err=%v", output, skipped, err)
	}
}

func TestHookDeadlineIsBounded(t *testing.T) {
	old := hookDeadline
	hookDeadline = 25 * time.Millisecond
	t.Cleanup(func() { hookDeadline = old })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := time.Now()
	_, skipped, err := RunHookBounded(func() (string, error) {
		<-release
		return "late", nil
	})
	if err != nil || skipped == "" {
		t.Fatalf("hook deadline did not degrade: skipped=%q err=%v", skipped, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("hook did not return promptly: %s", elapsed)
	}
}
