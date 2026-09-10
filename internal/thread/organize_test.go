package thread

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrganizeEventUsesOnlyOriginalText(t *testing.T) {
	s := testStore(t)
	e := testEvent()
	e.Text = "The cache may be stale."
	e.Data = map[string]any{"private": "opaque payload"}
	if _, err := s.CaptureEvent(e); err != nil {
		t.Fatal(err)
	}
	paths, err := s.Organize(eventNoteID(e.Source, e.ID), func(_ context.Context, prompt string) ([]byte, error) {
		if strings.Contains(prompt, "opaque payload") {
			t.Fatal("sent opaque event data to model")
		}
		return []byte(`{"items":[{"kind":"assumption","title":"Cache may be stale","next":"","evidence":"The cache may be stale."}]}`), nil
	})
	if err != nil || len(paths) != 1 {
		t.Fatalf("interpretation: %v %v", paths, err)
	}
	notes, err := s.Notes()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Kind == "assumption" && (n.Status != "suggested" || len(n.Related) != 1) {
			t.Fatalf("lost provenance or promoted claim: %+v", n)
		}
	}
}

func writeState(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestOrganizePreservesCaptureAndRequiresEvidence(t *testing.T) {
	s := testStore(t)
	n := NewNote("capture", "Call")
	n.Domain = "work"
	n.Body = "\nWe need an export.\n"
	s.New(n)
	runner := func(context.Context, string) ([]byte, error) {
		return []byte(`{"items":[{"kind":"action","title":"Explore export","next":"Clarify format","evidence":"We need an export."}]}`), nil
	}
	paths, err := s.Organize(n.ID, runner)
	if err != nil || len(paths) != 1 {
		t.Fatalf("%v %v", paths, err)
	}
	s.Organize(n.ID, runner)
	ns, _ := s.Notes()
	if len(ns) != 2 {
		t.Fatal("duplicates")
	}
	for _, item := range ns {
		if item.ID != n.ID && (item.Status != "suggested" || item.Domain != "work") {
			t.Fatal("promoted or misfiled suggestion")
		}
	}
	if _, err = s.Organize(n.ID, func(context.Context, string) ([]byte, error) { return nil, errors.New("offline") }); err == nil {
		t.Fatal("hidden failure")
	}
	got, _ := s.Find(n.ID)
	if got.Body != n.Body {
		t.Fatal("lost original")
	}
	_, err = s.Organize(n.ID, func(context.Context, string) ([]byte, error) {
		return []byte(`{"items":[{"kind":"action","title":"False","next":"","evidence":"not said"}]}`), nil
	})
	if err == nil {
		t.Fatal("accepted fabricated evidence")
	}
}
func TestPraxisTerminalStateNames(t *testing.T) {
	s := testStore(t)
	n := NewNote("project", "Praxis")
	n.ID = "praxis"
	s.New(n)
	// The contract uses underscores; keep imports faithful to the runtime.
	path := writeState(t, `{"spec_version":"1.0.0","run_id":"r","cursors":{"a":{"node_id":"a","status":"terminal_success"}},"last_applied_seq":1}`)
	if _, err := s.ImportRun("praxis", path, "praxis"); err != nil {
		t.Fatal(err)
	}
	ns, _ := s.Notes()
	r := LatestRuns(ns)[0]
	if r.Status != "done" || r.Metrics["successful_nodes"] != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestMissingDevelopMeasurementsStayUnknown(t *testing.T) {
	s := testStore(t)
	n := NewNote("project", "Demo")
	n.ID = "demo"
	s.New(n)
	path := writeState(t, `{"run_id":"r","node":"scan","status":"running"}`)
	if _, err := s.ImportRun("develop", path, "demo"); err != nil {
		t.Fatal(err)
	}
	notes, _ := s.Notes()
	r := LatestRuns(notes)[0]
	if _, ok := r.Metrics["completed_tasks"]; ok {
		t.Fatal("invented zero for absent tasks")
	}
}
