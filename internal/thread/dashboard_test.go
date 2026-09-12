package thread

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardRendersDateOrderedSessionEvidence(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	project, err := s.Project(project.ID)
	if err != nil {
		t.Fatal(err)
	}

	generic := NewNote("session", "Planning session")
	generic.Project = s.Link(project)
	generic.Created = "2026-09-10T09:00:00Z"
	// Session ordering is based on recorded creation time, not a later edit.
	generic.Updated = "2026-09-12T09:00:00Z"
	generic.Body = "\nRecorded plan: verify the migration.\n"
	if _, err := s.New(generic); err != nil {
		t.Fatal(err)
	}
	genericEvidence, err := s.Find(generic.ID)
	if err != nil {
		t.Fatal(err)
	}

	event := testEvent()
	event.ProjectID = project.ID
	event.OccurredAt = "2026-09-11T10:00:00Z"
	event.Text = "Recorded lifecycle evidence"
	if _, err := s.CaptureEvent(event); err != nil {
		t.Fatal(err)
	}
	evidence, err := s.Find(eventNoteID(event.Source, event.ID))
	if err != nil {
		t.Fatal(err)
	}

	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(s.Root, "Thread", "Overview.md") {
		t.Fatalf("dashboard path = %q", result.Path)
	}
	if result.Warning.Skipped != 0 || len(result.Warning.Paths) != 0 {
		t.Fatalf("complete vault returned warning: %+v", result.Warning)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"Atlas",
		"2026-09-10",
		"Planning session",
		"Recorded plan: verify the migration.",
		"[[" + genericEvidence.ID + "|Planning session]]",
		"2026-09-11",
		"Recorded lifecycle evidence",
		"[[" + evidence.ID + "|" + evidence.Title + "]]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dashboard omitted recorded session evidence %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "2026-09-10") > strings.Index(text, "2026-09-11") {
		t.Fatalf("sessions are not date ordered:\n%s", text)
	}
}

func TestDashboardOrdersEqualTimestampSessionEvidenceByStableNoteID(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	project, err := s.Project(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	newSession := func(id, title, updated, body string) Note {
		n := NewNote("session", title)
		n.ID, n.Project, n.Created, n.Updated, n.Body = id, s.Link(project), "2026-09-10T09:00:00Z", updated, body
		return n
	}
	// AvailableNotes orders its input by updated time. Deliberately create and
	// retain the reverse of the required ID order, so a comparator that merely
	// returns false would render Zeta before Alpha.
	sessions := []Note{
		newSession("session-zeta", "Zeta session", "2026-09-12T09:00:00Z", "\nZeta evidence\n"),
		newSession("session-alpha", "Alpha session", "2026-09-11T09:00:00Z", "\nAlpha evidence\n"),
	}
	for _, session := range sessions {
		if _, err := s.New(session); err != nil {
			t.Fatal(err)
		}
	}

	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	history := dashboardSection(t, string(body), "## Project session history", "## Memories / Unassigned")
	alpha := strings.Index(history, "[[session-alpha|Alpha session]]")
	zeta := strings.Index(history, "[[session-zeta|Zeta session]]")
	if alpha < 0 || zeta < 0 || alpha > zeta {
		t.Fatalf("equal-timestamp sessions were not ordered by stable note ID:\n%s", history)
	}
}

func TestDashboardExcludesNonSessionEventsFromProjectSessionHistory(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	nonSession := testEvent()
	nonSession.ID = "prompt-evidence"
	nonSession.Type = "prompt.completed"
	nonSession.ProjectID = project.ID
	nonSession.Text = "Prompt evidence must not be shown as a session"
	if _, err := s.CaptureEvent(nonSession); err != nil {
		t.Fatal(err)
	}

	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	history := dashboardSection(t, string(body), "## Project session history", "## Memories / Unassigned")
	if strings.Contains(history, "Prompt evidence must not be shown as a session") || strings.Contains(history, eventNoteID(nonSession.Source, nonSession.ID)) {
		t.Fatalf("non-session event appeared in project session history:\n%s", history)
	}
}

func TestDashboardReturnsBoundedPartialVaultWarning(t *testing.T) {
	s := testStore(t)
	available := NewNote("memory", "Available memory")
	missing := NewNote("memory", "Evicted memory")
	if _, err := s.New(available); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(missing); err != nil {
		t.Fatal(err)
	}
	original := datalessVaultFile
	datalessVaultFile = func(info os.FileInfo) bool { return info.Name() == missing.ID+".md" }
	t.Cleanup(func() { datalessVaultFile = original })

	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning.Skipped != 1 || len(result.Warning.Paths) != 1 || !strings.HasSuffix(result.Warning.Paths[0], missing.ID+".md") {
		t.Fatalf("dashboard concealed partial vault: %+v", result.Warning)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), available.Title) || strings.Contains(string(body), missing.Title) {
		t.Fatalf("dashboard did not render only locally available records:\n%s", body)
	}
}

func TestDashboardRendersHumanFacingCategoriesWithoutManagedLinks(t *testing.T) {
	s := testStore(t)
	project := NewNote("project", "Atlas")
	project.ID = "atlas"
	if _, err := s.New(project); err != nil {
		t.Fatal(err)
	}
	decision := NewNote("decision", "Use append-only records")
	discovery := NewNote("discovery", "The vault is local-first")
	memory := NewNote("memory", "Follow up with Dana")
	for _, note := range []Note{decision, discovery, memory} {
		if _, err := s.New(note); err != nil {
			t.Fatal(err)
		}
	}

	result, err := s.Dashboard("")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"## Projects",
		"## Memories / Unassigned",
		"## Decisions",
		"## Discoveries",
		project.Title,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("dashboard omitted human-facing navigation content %q:\n%s", want, text)
		}
	}
	memories := dashboardSection(t, text, "## Memories / Unassigned", "## Decisions")
	if !strings.Contains(memories, "[["+memory.ID+"|Follow up with Dana]]") {
		t.Errorf("unassigned memories section omitted memory link:\n%s", memories)
	}
	decisions := dashboardSection(t, text, "## Decisions", "## Discoveries")
	if !strings.Contains(decisions, "[["+decision.ID+"|Use append-only records]]") {
		t.Errorf("decisions section omitted decision link:\n%s", decisions)
	}
	discoveries := dashboardSection(t, text, "## Discoveries", "## Work to resume")
	if !strings.Contains(discoveries, "[["+discovery.ID+"|The vault is local-first]]") {
		t.Errorf("discoveries section omitted discovery link:\n%s", discoveries)
	}
	if strings.Contains(text, "[[Thread/.") {
		t.Fatalf("dashboard exposed a managed dot-directory as a graph-facing destination:\n%s", text)
	}
}

func dashboardSection(t *testing.T, text, heading, nextHeading string) string {
	t.Helper()
	start := strings.Index(text, heading)
	if start < 0 {
		t.Fatalf("dashboard omitted section %q:\n%s", heading, text)
	}
	section := text[start+len(heading):]
	end := strings.Index(section, nextHeading)
	if end < 0 {
		t.Fatalf("dashboard section %q has no following section %q:\n%s", heading, nextHeading, text)
	}
	return section[:end]
}
