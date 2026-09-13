package thread

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardCreatesCategoryProjectAndDatedSessionPages(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	project, err := s.Project("atlas")
	if err != nil {
		t.Fatal(err)
	}
	action := NewNote("action", "Ship the dashboard")
	action.Project = s.Link(project)
	action.Status = "active"
	action.Next = "Run tests"
	memory := NewNote("memory", "Keep original wording")
	memory.Project = s.Link(project)
	unassigned := NewNote("memory", "Ask Dana about dates")
	decision := NewNote("decision", "Use generated pages")
	decision.Project = s.Link(project)
	discovery := NewNote("discovery", "Obsidian owns graph settings")
	discovery.Project = s.Link(project)
	session := NewNote("session", "Planning session")
	session.Project = s.Link(project)
	session.Created = "2026-09-10T09:00:00Z"
	session.Updated = session.Created
	session.Body = "\nRecorded plan: verify the migration.\n"
	for _, n := range []Note{action, memory, unassigned, decision, discovery, session} {
		if _, err := s.New(n); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) < 10 {
		t.Fatalf("generated paths = %v", result.Paths)
	}
	pages := map[string][]string{
		"Thread/Dashboards/Projects.md": {"Atlas", "dashboard"}, "Thread/Dashboards/Todos.md": {"Ship the dashboard"},
		"Thread/Dashboards/Memories.md": {"Ask Dana about dates"}, "Thread/Dashboards/Decisions.md": {"Use generated pages"},
		"Thread/Dashboards/Discoveries.md": {"Obsidian owns graph settings"}, "Thread/Projects/atlas/Dashboard.md": {"Ship the dashboard", "Keep original wording", "Use generated pages", "Obsidian owns graph settings", "2026-09-10"},
		"Thread/Projects/atlas/Sessions/2026-09-10/summary.md": {"Planning session", "Recorded plan: verify the migration."},
	}
	for rel, wants := range pages {
		body, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
		if !strings.HasPrefix(string(body), generatedMarker) {
			t.Fatalf("%s lacks ownership marker", rel)
		}
		for _, want := range wants {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s omitted %q:\n%s", rel, want, body)
			}
		}
	}
	graph, err := os.ReadFile(filepath.Join(s.Root, "Thread/Dashboards/Graph.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(graph), "-path:Thread/.items") {
		t.Fatalf("graph setup omitted data filter: %s", graph)
	}
}

func TestDashboardResetsOwnedPagesAndRemovesStaleSessionSummary(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	project, _ = s.Project("atlas")
	session := NewNote("session", "Old session")
	session.Project = s.Link(project)
	session.Created, session.Updated = "2026-09-10T09:00:00Z", "2026-09-10T09:00:00Z"
	if _, err := s.New(session); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dashboard(""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, "Thread/Projects/atlas/Sessions/2026-09-10/summary.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Find(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stored.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dashboard(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stale generated summary was retained: %v", err)
	}
}

func TestDashboardMigratesLegacyOverviewButProtectsUserPages(t *testing.T) {
	s := testStore(t)
	legacy := filepath.Join(s.Root, "Thread/Overview.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("<!-- thread-generated-overview -->\nold"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dashboard(""); err != nil {
		t.Fatalf("legacy dashboard should migrate: %v", err)
	}
	user := filepath.Join(s.Root, "Thread/Dashboards/Todos.md")
	if err := os.WriteFile(user, []byte("my todos"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dashboard(""); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("user dashboard ownership = %v", err)
	}
}
