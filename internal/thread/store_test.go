package thread

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewCaptureAlwaysUsesHiddenItemsWithLegacyFolderPresent(t *testing.T) {
	s := testStore(t)
	legacy := filepath.Join(s.Root, "Thread", "Items")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	n := NewNote("capture", "Canonical storage regression")
	path, err := s.New(n)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(s.Root, "Thread", ".items", n.ID+".md") {
		t.Fatalf("capture used noncanonical path: %s", path)
	}
	entries, err := os.ReadDir(legacy)
	if err != nil || len(entries) != 0 {
		t.Fatalf("capture wrote into legacy storage: %v %v", entries, err)
	}
}
func TestCaptureConcurrentAndUpdatePreservesUserContent(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := NewNote("capture", "A conference call: X, Y, Z")
			n.Body = "\nExact wording\n"
			if _, err := s.New(n); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	ns, err := s.Notes()
	if err != nil || len(ns) != 30 {
		t.Fatalf("%d %v", len(ns), err)
	}
	n := ns[0]
	n.Extra = map[string]any{"owner": "Dana", "custom_list": []string{"one", "two"}}
	n.Body = "\nMy hand-written notes.\n"
	b, _ := Encode(n)
	if err = os.WriteFile(n.Path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Update(n.ID, func(x *Note) error { x.Status = "active"; x.Next = "Ask Dana"; return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != n.Body || got.Extra["owner"] != "Dana" || got.Next != "Ask Dana" {
		t.Fatalf("lost user data: %+v", got)
	}
	history, _ := filepath.Glob(filepath.Join(s.Root, "Thread/.history", n.ID, "*.md"))
	if len(history) != 1 {
		t.Fatal("missing history")
	}
	old, _ := os.ReadFile(history[0])
	if string(old) != string(b) {
		t.Fatal("history is not exact original")
	}
}
func TestDuplicateAndSymlinkNeverOverwrite(t *testing.T) {
	s := testStore(t)
	n := NewNote("capture", "Original")
	p, err := s.New(n)
	if err != nil {
		t.Fatal(err)
	}
	n.Title = "Replacement"
	if _, err = s.New(n); !os.IsExist(err) {
		t.Fatalf("expected exists: %v", err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "Replacement") {
		t.Fatal("overwritten")
	}
	other := testStore(t)
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(other.Root, "Thread")); err != nil {
		t.Fatal(err)
	}
	if _, err = other.New(NewNote("capture", "No escape")); err == nil {
		t.Fatal("followed symlink")
	}
	n.ID = "../escape"
	if _, err = s.New(n); err == nil {
		t.Fatal("accepted traversal")
	}
}
func TestCorruptionAndConflictsAreVisible(t *testing.T) {
	s := testStore(t)
	n := NewNote("action", "Test")
	p, _ := s.New(n)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(filepath.Join(filepath.Dir(p), "conflicted-copy.md"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Notes(); err == nil {
		t.Fatal("duplicate ID hidden")
	}
	s2 := testStore(t)
	p, _ = s2.New(NewNote("capture", "test"))
	os.WriteFile(p, []byte("broken"), 0600)
	if _, err := s2.Notes(); err == nil {
		t.Fatal("corrupt note hidden")
	}
}
func TestRunImportIdempotentAndLatestOnly(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Praxis")
	project.ID = "praxis"
	s.New(project)
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"run_id":"one","node":"bundle_scheduler","status":"running","handoffs":2,"tasks_runtime":{"a":{"status":"complete"}}}`), 0600)
	first, err := s.ImportRun("develop", path, "praxis")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ImportRun("develop", path, "praxis")
	if err != nil || first != second {
		t.Fatalf("not idempotent %s %s %v", first, second, err)
	}
	os.WriteFile(path, []byte(`{"run_id":"one","node":"complete","status":"complete","handoffs":3,"tasks_runtime":{"a":{"status":"complete"},"b":{"status":"complete"}}}`), 0600)
	if _, err = s.ImportRun("develop", path, "praxis"); err != nil {
		t.Fatal(err)
	}
	notes, _ := s.Notes()
	runs := LatestRuns(notes)
	if len(runs) != 1 || runs[0].Metrics["handoffs"] != 3 || runs[0].Metrics["completed_tasks"] != 2 {
		t.Fatalf("wrong aggregation %+v", runs)
	}
	if _, ok := runs[0].Metrics["tokens"]; ok {
		t.Fatal("invented token usage")
	}
}
func TestInitPreservesExistingAndDashboardOwnership(t *testing.T) {
	s := testStore(t)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	guide := filepath.Join(s.Root, "Thread/Guide.md")
	os.WriteFile(guide, []byte("my guide"), 0600)
	s.Init()
	b, _ := os.ReadFile(guide)
	if string(b) != "my guide" {
		t.Fatal("overwrote guide")
	}
	overview := filepath.Join(s.Root, "Thread/Overview.md")
	os.WriteFile(overview, []byte("my own page"), 0600)
	if _, err := s.Dashboard(); err == nil {
		t.Fatal("overwrote user overview")
	}
}
