package thread

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
)

func testEvent() Event {
	return Event{Schema: 1, ID: "delivery-1", Source: "fixture", Type: "session.started", SessionID: "s1", OccurredAt: "2026-09-09T10:00:00Z", ProjectID: "offline", Repo: "/unavailable/repository", Text: "Exact wording\nwith a second line"}
}

func TestEventConcurrentRetriesAndConflicts(t *testing.T) {
	s := testStore(t)
	e := testEvent()
	e.Data = map[string]any{"tokens": json.Number("9007199254740993")}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.CaptureEvent(e); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	notes, err := s.Notes()
	if err != nil || len(notes) != 1 {
		t.Fatalf("capture: %d, %v", len(notes), err)
	}
	before, err := os.ReadFile(notes[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := noteEvent(notes[0])
	if err != nil || got.Text != e.Text || got.Data["tokens"] != json.Number("9007199254740993") {
		t.Fatalf("lost evidence: %+v, %v", got, err)
	}
	e.Text = "different"
	if _, err := s.CaptureEvent(e); err == nil {
		t.Fatal("accepted conflicting identity")
	}
	if err := s.Update(notes[0].ID, func(n *Note) error { n.Status = "done"; return nil }); err == nil {
		t.Fatal("updated immutable event")
	}
	after, _ := os.ReadFile(notes[0].Path)
	if string(before) != string(after) {
		t.Fatal("original changed")
	}
}

func TestDecodeEventValidation(t *testing.T) {
	for name := range eventTypes {
		e := testEvent()
		e.Type = name
		raw, _ := json.Marshal(e)
		if _, err := DecodeEvent(raw); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"schema":2}`, strings.Repeat("x", MaxEventBytes+1)} {
		if _, err := DecodeEvent([]byte(raw)); err == nil {
			t.Fatal("accepted invalid event")
		}
	}
	e := testEvent()
	raw, _ := json.Marshal(e)
	if _, err := DecodeEvent(append(raw, []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing JSON")
	}
	e.SessionID = ""
	raw, _ = json.Marshal(e)
	if _, err := DecodeEvent(raw); err == nil {
		t.Fatal("accepted lifecycle without session")
	}
}

func TestOrientationOfflineBoundedAndCheckpoint(t *testing.T) {
	s := testStore(t)
	p := NewNote("project", "Offline project")
	p.ID = "offline"
	p.Repo = "/unavailable/repository"
	if _, err := s.New(p); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Project(p.ID)
	e := testEvent()
	e.ID = "stop"
	e.Type = "session.stopped"
	e.OccurredAt = "2026-09-09T11:00:00Z"
	if _, err := s.CaptureEvent(e); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureEvent(testEvent()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		n := NewNote("action", "Saved work")
		n.Project = s.Link(p)
		n.Status = "paused"
		n.Next = "Read the failure log"
		if _, err := s.New(n); err != nil {
			t.Fatal(err)
		}
	}
	e.ID = "tool"
	e.Type = "tool.after"
	e.Data = map[string]any{"opaque": "DO NOT DUMP THIS"}
	if _, err := s.CaptureEvent(e); err != nil {
		t.Fatal(err)
	}
	o, err := s.Orient(p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Work) != 2 || o.Omitted["work"] != 2 || len(o.Sessions) != 1 || o.Sessions[0].State != "stopped" || o.Omitted["low_level_events"] != 1 {
		t.Fatalf("bad brief: %+v", o)
	}
	if len(o.Attention.NeedsAttention) != 0 || len(o.Attention.CanSetDown) != 2 {
		t.Fatalf("bad attention: %+v", o.Attention)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "DO NOT DUMP") {
		t.Fatal("opaque data leaked into brief")
	}
	first, err := s.Checkpoint(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Checkpoint(p.ID)
	if err != nil || first != second {
		t.Fatalf("checkpoint not idempotent: %s %s %v", first, second, err)
	}
	n := NewNote("risk", "Unresolved blocker")
	n.Status = "blocked"
	n.Project = s.Link(p)
	if _, err := s.New(n); err != nil {
		t.Fatal(err)
	}
	third, err := s.Checkpoint(p.ID)
	if err != nil || third == first {
		t.Fatalf("new state did not create checkpoint: %v", err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("historical checkpoint lost", err)
	}
}
