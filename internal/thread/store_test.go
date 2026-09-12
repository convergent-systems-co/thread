package thread

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestVaultReadsHaveOneBoundedDeadline(t *testing.T) {
	s := testStore(t)
	n := NewNote("capture", "Blocking record")
	if _, err := s.New(n); err != nil {
		t.Fatal(err)
	}
	originalRead, originalTimeout := readVaultFile, vaultReadTimeout
	blocked := make(chan struct{})
	readVaultFile = func(path string) ([]byte, error) {
		<-blocked
		return originalRead(path)
	}
	vaultReadTimeout = 25 * time.Millisecond
	t.Cleanup(func() {
		close(blocked)
		readVaultFile, vaultReadTimeout = originalRead, originalTimeout
	})
	started := time.Now()
	_, err := s.Notes()
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || !IsVaultUnavailable(err) {
		t.Fatalf("expected bounded vault timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("vault read was not bounded: %s", elapsed)
	}
}

func TestAvailableNotesSkipDatalessFilesWithWarning(t *testing.T) {
	s := testStore(t)
	available := NewNote("capture", "Available")
	missing := NewNote("capture", "Evicted")
	if _, err := s.New(available); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(missing); err != nil {
		t.Fatal(err)
	}
	original := datalessVaultFile
	datalessVaultFile = func(info os.FileInfo) bool { return info.Name() == missing.ID+".md" }
	t.Cleanup(func() { datalessVaultFile = original })
	notes, warning, err := s.AvailableNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].ID != available.ID || warning.Skipped != 1 || len(warning.Paths) != 1 {
		t.Fatalf("unexpected partial result: notes=%+v warning=%+v", notes, warning)
	}
	if _, err = s.Notes(); err == nil || !IsVaultUnavailable(err) {
		t.Fatalf("complete read silently ignored dataless record: %v", err)
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
	if _, err := s.Dashboard(""); err == nil {
		t.Fatal("overwrote user overview")
	}
}

func TestDashboardFiltersDomain(t *testing.T) {
	s := testStore(t)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		title  string
		domain string
	}{
		{"Home work", "home"},
		{"Work work", "work"},
	} {
		n := NewNote("action", item.title)
		n.Domain = item.domain
		n.Status = "active"
		n.Next = "Continue"
		if _, err := s.New(n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Dashboard("home"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(s.Root, "Thread", "Overview.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Home work") || strings.Contains(string(body), "Work work") {
		t.Fatalf("dashboard did not filter domain: %s", body)
	}
	if _, err := s.Dashboard("other"); err == nil {
		t.Fatal("accepted invalid dashboard domain")
	}
}
