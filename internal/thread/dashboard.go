package thread

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Bases intentionally excludes Thread's implementation folders. Obsidian's
// Graph filter remains a user workspace preference, which Thread must not own.
const bases = `filters:
  and:
    - 'file.inFolder("Thread")'
    - '!file.inFolder("Thread/.items")'
    - '!file.inFolder("Thread/.runs")'
    - '!file.inFolder("Thread/.events")'
    - '!file.inFolder("Thread/.history")'
    - 'thread_schema == 1'
views:
  - type: table
    name: Projects
    filters: {and: ['kind == "project"']}
    order: [title, status, domain, updated]
  - type: table
    name: Todos
    filters: {and: ['status == "active" || status == "paused" || status == "blocked"', 'kind != "project" && kind != "run" && kind != "event" && kind != "checkpoint"']}
    order: [title, project, status, next, updated]
  - type: table
    name: Memories / Unassigned
    filters: {and: ['kind == "memory"', 'project == null']}
    order: [title, source, created]
  - type: table
    name: Decisions
    filters: {and: ['kind == "decision"']}
    order: [title, project, updated]
  - type: table
    name: Discoveries
    filters: {and: ['kind == "discovery"']}
    order: [title, project, updated]
  - type: table
    name: Sessions
    filters: {and: ['kind == "session"']}
    order: [title, project, created, updated]
`

const generatedMarker = "<!-- thread-generated-dashboard -->\n"

func (s *Store) Init() error {
	content := map[string]string{
		"Thread/Thread.base":   bases,
		"Thread/Start Here.md": "# Thread\n\nA place to put work down and pick it up again.\n\n![[Thread/Thread.base]]\n\n## Dashboards\n\n- [[Thread/Dashboards/Projects|Projects]]\n- [[Thread/Dashboards/Todos|Todos]]\n- [[Thread/Dashboards/Memories|Memories / Unassigned]]\n- [[Thread/Dashboards/Decisions|Decisions]]\n- [[Thread/Dashboards/Discoveries|Discoveries]]\n- [[Thread/Dashboards/Sessions|Sessions]]\n\n[[Thread/Overview|Dashboard home]] · [[Thread/Guide|How Thread works]]\n",
		"Thread/Guide.md":      "# Using Thread\n\nCapture first; organize later. Edit item properties in Obsidian: `status`, `next`, `tags`, `related`, and `domain`. CLI updates preserve extra properties and body text.\n\n`thread dashboard` resets Thread-owned dashboard pages: the dashboard home, category pages, each project dashboard, and date-based session summaries. It never replaces project notes or other user-owned pages.\n\nOpen [[Thread/Dashboards/Graph|Graph setup]] once to hide Thread's dot-prefixed implementation folders in Obsidian's Graph view. That preference belongs to Obsidian, so Thread does not overwrite your vault settings.\n\n`.items/`, `.runs/`, `.events/`, and `.history/` are Thread-managed storage. Do not move or rename files there manually.\n",
	}
	for rel, body := range content {
		p, err := s.path(rel)
		if err != nil {
			return err
		}
		if err = s.create(p, []byte(body)); err != nil && !os.IsExist(err) {
			return err
		}
	}
	for _, rel := range []string{"Thread/Projects", "Thread/Dashboards"} {
		p, err := s.path(rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(p, 0700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Overview(domain string) (string, error) {
	notes, err := s.Notes()
	if err != nil {
		return "", err
	}
	return s.renderHome(notes, domain, VaultReadWarning{}), nil
}

type sessionEvidence struct {
	note          Note
	occurred      time.Time
	date, summary string
}

func projectSessionEvidence(project Note, notes []Note, filter func(Note) bool) []sessionEvidence {
	var evidence []sessionEvidence
	for _, n := range notes {
		if !filter(n) || !SameLink(n.Project, project.PathLink()) {
			continue
		}
		if n.Kind == "session" {
			if occurred, err := time.Parse(time.RFC3339Nano, n.Created); err == nil {
				evidence = append(evidence, sessionEvidence{n, occurred, occurred.Format("2006-01-02"), noteSummary(n)})
			}
			continue
		}
		if n.Kind != "event" || !strings.HasPrefix(str(n.Extra["event_type"]), "session.") {
			continue
		}
		occurred, err := time.Parse(time.RFC3339Nano, str(n.Extra["occurred_at"]))
		if err != nil {
			continue
		}
		summary := noteSummary(n)
		if event, err := noteEvent(n); err == nil && strings.TrimSpace(event.Text) != "" {
			summary = strings.TrimSpace(event.Text)
		}
		evidence = append(evidence, sessionEvidence{n, occurred, occurred.Format("2006-01-02"), compact(summary, 240)})
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].occurred.Equal(evidence[j].occurred) {
			return evidence[i].note.ID < evidence[j].note.ID
		}
		return evidence[i].occurred.Before(evidence[j].occurred)
	})
	return evidence
}

func (s *Store) dashboardLink(n Note) string {
	if n.Kind == "project" {
		return s.Link(n)
	}
	return "[[" + n.ID + "|" + strings.ReplaceAll(strings.ReplaceAll(n.Title, "|", "-"), "]", "") + "]]"
}
func (n Note) PathLink() string {
	return "[[" + strings.TrimSuffix(filepath.ToSlash(filepath.Join("Thread", "Projects", n.ID)), ".md") + "]]"
}
func noteSummary(n Note) string {
	for _, line := range strings.Split(strings.TrimSpace(n.Body), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return compact(line, 240)
		}
	}
	return compact(n.Title, 240)
}
func compact(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit-1]) + "…"
}
func sortedNotes(notes []Note) {
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].Updated == notes[j].Updated {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].Updated > notes[j].Updated
	})
}
func projectDashboardPath(id string) string {
	return filepath.ToSlash(filepath.Join("Thread", "Projects", id, "Dashboard.md"))
}
func sessionSummaryPath(id, date string) string {
	return filepath.ToSlash(filepath.Join("Thread", "Projects", id, "Sessions", date, "summary.md"))
}
func dashboardLinkForPath(path, label string) string {
	return "[[" + strings.TrimSuffix(filepath.ToSlash(path), ".md") + "|" + label + "]]"
}

func (s *Store) renderHome(notes []Note, domain string, warning VaultReadWarning) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Thread dashboards\n\nGenerated %s. This is an index of regenerated human-facing views; source records remain unchanged.\n\n", Now())
	if warning.Skipped > 0 {
		fmt.Fprintf(&b, "> [!warning] Partial vault view\n> %s\n\n", warning.Error())
	}
	b.WriteString("## Open a dashboard\n\n- [[Thread/Dashboards/Projects|Projects]]\n- [[Thread/Dashboards/Todos|Todos]]\n- [[Thread/Dashboards/Memories|Memories / Unassigned]]\n- [[Thread/Dashboards/Decisions|Decisions]]\n- [[Thread/Dashboards/Discoveries|Discoveries]]\n- [[Thread/Dashboards/Sessions|Sessions]]\n\n## Work to resume\n\n")
	active := filterNotes(notes, func(n Note) bool {
		return (domain == "" || n.Domain == domain) && n.Kind != "project" && n.Kind != "run" && n.Kind != "event" && n.Kind != "checkpoint" && (n.Status == "active" || n.Status == "paused" || n.Status == "blocked")
	})
	if len(active) == 0 {
		b.WriteString("No active, paused, or blocked work recorded.\n")
	} else {
		if len(active) > 5 {
			active = active[:5]
		}
		writeNoteList(&b, active)
	}
	b.WriteString("\n[[Thread/Dashboards/Graph|Graph setup]]\n")
	return b.String()
}

func filterNotes(notes []Note, fn func(Note) bool) []Note {
	out := []Note{}
	for _, n := range notes {
		if fn(n) {
			out = append(out, n)
		}
	}
	sortedNotes(out)
	return out
}
func writeNoteList(b *strings.Builder, notes []Note) {
	if len(notes) == 0 {
		b.WriteString("No records yet.\n")
		return
	}
	for _, n := range notes {
		fmt.Fprintf(b, "- %s\n", sDashboardLink(n))
	}
}
func sDashboardLink(n Note) string {
	if n.Kind == "project" {
		return "[[Thread/Projects/" + n.ID + "|" + n.Title + "]]"
	}
	return "[[" + n.ID + "|" + strings.ReplaceAll(strings.ReplaceAll(n.Title, "|", "-"), "]", "") + "]]"
}
func (s *Store) categoryPage(title, intro string, notes []Note) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", title, intro)
	writeNoteList(&b, notes)
	return b.String()
}

func (s *Store) renderProjects(projects []Note) string {
	var b strings.Builder
	b.WriteString("# Projects\n\nEach project has a generated dashboard for its active work, captured types, and dated session summaries.\n\n")
	if len(projects) == 0 {
		b.WriteString("No projects recorded yet.\n")
		return b.String()
	}
	for _, p := range projects {
		fmt.Fprintf(&b, "- %s — %s\n", s.dashboardLink(p), dashboardLinkForPath(projectDashboardPath(p.ID), "dashboard"))
	}
	return b.String()
}

func (s *Store) renderProject(project Note, notes []Note, filter func(Note) bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s dashboard\n\nProject: %s\n\n", project.Title, s.dashboardLink(project))
	cats := []struct {
		title string
		notes []Note
	}{
		{"Todos", filterNotes(notes, func(n Note) bool {
			return filter(n) && SameLink(n.Project, project.PathLink()) && n.Kind != "project" && n.Kind != "run" && n.Kind != "event" && n.Kind != "checkpoint" && (n.Status == "active" || n.Status == "paused" || n.Status == "blocked")
		})},
		{"Memories", filterNotes(notes, func(n Note) bool { return filter(n) && SameLink(n.Project, project.PathLink()) && n.Kind == "memory" })},
		{"Decisions", filterNotes(notes, func(n Note) bool { return filter(n) && SameLink(n.Project, project.PathLink()) && n.Kind == "decision" })},
		{"Discoveries", filterNotes(notes, func(n Note) bool {
			return filter(n) && SameLink(n.Project, project.PathLink()) && n.Kind == "discovery"
		})},
	}
	for _, c := range cats {
		fmt.Fprintf(&b, "## %s\n\n", c.title)
		writeNoteList(&b, c.notes)
		b.WriteString("\n")
	}
	b.WriteString("## Sessions\n\n")
	evidence := projectSessionEvidence(project, notes, filter)
	dates := []string{}
	seen := map[string]bool{}
	for _, e := range evidence {
		if !seen[e.date] {
			seen[e.date] = true
			dates = append(dates, e.date)
		}
	}
	if len(dates) == 0 {
		b.WriteString("No session evidence recorded yet.\n")
	} else {
		for _, d := range dates {
			fmt.Fprintf(&b, "- %s\n", dashboardLinkForPath(sessionSummaryPath(project.ID, d), d))
		}
	}
	return b.String()
}

func (s *Store) renderSessionSummary(project Note, date string, evidence []sessionEvidence) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n\nProject: %s\n\nThis deterministic summary lists recorded session evidence for this date. It does not infer work that was not captured.\n\n## Evidence\n\n", project.Title, date, s.dashboardLink(project))
	for _, e := range evidence {
		fmt.Fprintf(&b, "- %s — %s\n", s.dashboardLink(e.note), e.summary)
	}
	return b.String()
}

func graphPage() string {
	return "# Graph setup\n\nThread keeps implementation records in dot-prefixed folders. In Obsidian’s **Graph view → Filters**, add:\n\n```text\n-path:Thread/.items -path:Thread/.runs -path:Thread/.events -path:Thread/.history\n```\n\nThis is an Obsidian workspace preference, so `thread dashboard` deliberately does not edit `.obsidian` settings or overwrite your graph configuration. The generated dashboards link to source records for traceability; the filter hides those data-only nodes from the graph.\n"
}

type dashboardOutput struct{ rel, body string }
type DashboardResult struct {
	Path    string
	Paths   []string
	Warning VaultReadWarning
}

func (s *Store) Dashboard(domain string) (DashboardResult, error) {
	if domain != "" && domain != "home" && domain != "work" && domain != "unknown" {
		return DashboardResult{}, fmt.Errorf("invalid domain %q", domain)
	}
	notes, warning, err := s.AvailableNotes()
	if err != nil {
		return DashboardResult{}, err
	}
	filter := func(n Note) bool { return domain == "" || n.Domain == domain }
	projects := filterNotes(notes, func(n Note) bool { return filter(n) && n.Kind == "project" })
	outputs := []dashboardOutput{
		{"Thread/Overview.md", s.renderHome(notes, domain, warning)}, {"Thread/Dashboards/Projects.md", s.renderProjects(projects)},
		{"Thread/Dashboards/Todos.md", s.categoryPage("Todos", "Active, paused, and blocked work across projects.", filterNotes(notes, func(n Note) bool {
			return filter(n) && n.Kind != "project" && n.Kind != "run" && n.Kind != "event" && n.Kind != "checkpoint" && (n.Status == "active" || n.Status == "paused" || n.Status == "blocked")
		}))},
		{"Thread/Dashboards/Memories.md", s.categoryPage("Memories / Unassigned", "Memories without a project are shown here. Project-linked memories remain on their project dashboard.", filterNotes(notes, func(n Note) bool { return filter(n) && n.Kind == "memory" && n.Project == "" }))},
		{"Thread/Dashboards/Decisions.md", s.categoryPage("Decisions", "Recorded decisions, linked to their source records.", filterNotes(notes, func(n Note) bool { return filter(n) && n.Kind == "decision" }))},
		{"Thread/Dashboards/Discoveries.md", s.categoryPage("Discoveries", "Recorded discoveries, linked to their source records.", filterNotes(notes, func(n Note) bool { return filter(n) && n.Kind == "discovery" }))}, {"Thread/Dashboards/Graph.md", graphPage()},
	}
	type projectEvidence struct {
		project  Note
		evidence sessionEvidence
	}
	all := []projectEvidence{}
	for _, p := range projects {
		outputs = append(outputs, dashboardOutput{projectDashboardPath(p.ID), s.renderProject(p, notes, filter)})
		for _, e := range projectSessionEvidence(p, notes, filter) {
			all = append(all, projectEvidence{p, e})
		}
	}
	byProjectDate := map[string][]sessionEvidence{}
	for _, x := range all {
		byProjectDate[x.project.ID+"/"+x.evidence.date] = append(byProjectDate[x.project.ID+"/"+x.evidence.date], x.evidence)
	}
	var index strings.Builder
	index.WriteString("# Sessions\n\nDated summaries are grouped under their project.\n\n")
	if len(all) == 0 {
		index.WriteString("No session evidence recorded yet.\n")
	}
	for _, p := range projects {
		dates := []string{}
		for key := range byProjectDate {
			if strings.HasPrefix(key, p.ID+"/") {
				dates = append(dates, strings.TrimPrefix(key, p.ID+"/"))
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dates)))
		if len(dates) > 0 {
			fmt.Fprintf(&index, "## %s\n\n", s.dashboardLink(p))
			for _, d := range dates {
				fmt.Fprintf(&index, "- %s\n", dashboardLinkForPath(sessionSummaryPath(p.ID, d), d))
				outputs = append(outputs, dashboardOutput{sessionSummaryPath(p.ID, d), s.renderSessionSummary(p, d, byProjectDate[p.ID+"/"+d])})
			}
			index.WriteString("\n")
		}
	}
	outputs = append(outputs, dashboardOutput{"Thread/Dashboards/Sessions.md", index.String()})
	expected, paths := map[string]bool{}, make([]string, 0, len(outputs))
	for _, o := range outputs {
		expected[o.rel] = true
		p, err := s.writeDashboard(o.rel, o.body)
		if err != nil {
			return DashboardResult{}, err
		}
		paths = append(paths, p)
	}
	if err := s.removeStaleDashboards(expected); err != nil {
		return DashboardResult{}, err
	}
	return DashboardResult{Path: paths[0], Paths: paths, Warning: warning}, nil
}

func (s *Store) writeDashboard(rel, body string) (string, error) {
	p, err := s.path(rel)
	if err != nil {
		return "", err
	}
	old, err := os.ReadFile(p)
	if err == nil && !dashboardOwned(rel, string(old)) {
		return "", fmt.Errorf("%s exists without the generated marker; refusing to overwrite", p)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".thread-dashboard-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(generatedMarker + body); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return "", err
	}
	return p, syncDir(filepath.Dir(p))
}

func dashboardOwned(rel, body string) bool {
	return strings.HasPrefix(body, generatedMarker) || (rel == "Thread/Overview.md" && strings.HasPrefix(body, "<!-- thread-generated-overview -->\n"))
}

func (s *Store) removeStaleDashboards(expected map[string]bool) error {
	root, err := s.path("Thread")
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || (filepath.Base(path) != "summary.md" && filepath.Base(path) != "Dashboard.md") {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if expected[rel] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(body), generatedMarker) {
			return os.Remove(path)
		}
		return nil
	})
}
