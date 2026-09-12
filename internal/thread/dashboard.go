package thread

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const bases = `filters:
  and:
    - 'file.inFolder("Thread/.items") || file.inFolder("Thread/Projects")'
    - 'thread_schema == 1'
views:
  - type: table
    name: Resume work
    filters:
      and:
        - 'status == "active" || status == "paused" || status == "blocked"'
    order: [title, project, status, next, domain, updated]
  - type: table
    name: Inbox and suggestions
    filters:
      and:
        - 'status == "inbox" || status == "suggested"'
    order: [title, kind, project, source, domain, created]
  - type: table
    name: Decisions and memory
    filters:
      and:
        - 'kind == "decision" || kind == "memory" || kind == "habit" || kind == "discovery"'
    order: [title, kind, project, related, updated]
`

func (s *Store) Init() error {
	content := map[string]string{
		"Thread/Thread.base":   bases,
		"Thread/Start Here.md": "# Thread\n\nA place to put work down and pick it up again.\n\n![[Thread/Thread.base]]\n\n[[Thread/Overview|Latest overview]] · [[Thread/Guide|How Thread works]]\n",
		"Thread/Guide.md":      "# Using Thread\n\nCapture first; organize later. Edit item properties in Obsidian: `status`, `next`, `tags`, `related`, and `domain`. CLI updates preserve extra properties and body text.\n\nStatuses: inbox, suggested, active, paused, blocked, done, archived. Suggestions are not commitments. Set one concrete `next` action on active work.\n\nEach item links to its project. Use `related` links for dependencies and shared ideas; explain the relationship in the note body. The graph follows these links.\n\n`.items/` and `.runs/` are Thread-managed storage. `.items/` contains captures, sessions, and recovery records; `.runs/` contains immutable execution observations. Do not move, rename, or reorganize files there manually. Humans may correct item properties when needed; use the CLI for lifecycle changes.\n\nRun observations are immutable snapshots from develop/Praxis. Thread does not advance their execution state. The generated Overview is refreshed by `thread dashboard [--domain home|work|unknown]`; the command writes the Markdown view and does not open Obsidian. Human-facing work views live in Start Here, Overview, Thread.base, and project notes.\n\nCLI edits keep prior note versions in `Thread/.history`. This is local note history, not a separate backup or code protection. Avoid editing the same note simultaneously on multiple machines: iCloud is eventually consistent. Duplicate IDs and malformed notes are surfaced as errors.\n\nUse `thread snapshot --repo PATH` for a verified local working-file ZIP and `thread organize --id ID` for optional headless Claude classification. Automatic hooks, live sessions, and background polling are not enabled in this release. Unknown metrics are not zero, and session duration is not human working time.\n",
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
	return nil
}
func (s *Store) Overview(domain string) (string, error) {
	notes, err := s.Notes()
	if err != nil {
		return "", err
	}
	return s.renderOverview(notes, domain, VaultReadWarning{}), nil
}

func (s *Store) renderOverview(notes []Note, domain string, warning VaultReadWarning) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Thread overview\n\nGenerated %s. Source observations may be older.\n\n", Now())
	filter := func(n Note) bool { return domain == "" || n.Domain == domain }
	if warning.Skipped > 0 {
		fmt.Fprintf(&b, "> [!warning] Partial vault view\n> %s\n\n", warning.Error())
	}
	b.WriteString("## Navigation\n\n- [[Thread/Projects|Projects]]\n- [[Thread/Overview#Work to resume|Todos]]\n- [[Thread/Overview#Memories / Unassigned|Memories]]\n- [[Thread/Overview#Decisions|Decisions]]\n- [[Thread/Overview#Discoveries|Discoveries]]\n- [[Thread/Overview#Project session history|Sessions]]\n\n")
	b.WriteString("## Projects\n\n")
	projects := make([]Note, 0)
	for _, n := range notes {
		if n.Kind == "project" && filter(n) {
			projects = append(projects, n)
		}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].ID < projects[j].ID })
	if len(projects) == 0 {
		b.WriteString("No selected projects recorded yet.\n")
	} else {
		for _, project := range projects {
			fmt.Fprintf(&b, "- %s\n", s.dashboardLink(project))
		}
	}
	b.WriteString("\n## Project session history\n\n")
	b.WriteString("Generic session notes use their recorded `created` timestamp. Lifecycle-event evidence uses its recorded `occurred_at` timestamp.\n\n")
	for _, project := range projects {
		fmt.Fprintf(&b, "### %s\n\n", s.dashboardLink(project))
		s.renderProjectSessions(&b, project, notes, filter)
	}
	if len(projects) == 0 {
		b.WriteString("No project session evidence recorded yet.\n")
	}
	b.WriteString("\n## Memories / Unassigned\n\n")
	memories := make([]Note, 0)
	for _, n := range notes {
		if filter(n) && n.Kind == "memory" && n.Project == "" {
			memories = append(memories, n)
		}
	}
	sort.Slice(memories, func(i, j int) bool { return memories[i].ID < memories[j].ID })
	if len(memories) == 0 {
		b.WriteString("No unassigned memories recorded yet.\n")
	} else {
		for _, memory := range memories {
			fmt.Fprintf(&b, "- %s\n", s.dashboardLink(memory))
		}
	}
	b.WriteString("\n## Decisions\n\n")
	s.renderCategory(&b, notes, filter, "decision")
	b.WriteString("\n## Discoveries\n\n")
	s.renderCategory(&b, notes, filter, "discovery")
	b.WriteString("## Work to resume\n\n")
	count := 0
	for _, n := range notes {
		if !filter(n) || n.Kind == "run" || n.Kind == "event" || n.Kind == "checkpoint" || n.Kind == "project" || n.Status == "done" || n.Status == "archived" || n.Status == "inbox" || n.Status == "suggested" {
			continue
		}
		count++
		fmt.Fprintf(&b, "- %s — %s; next: %s\n", s.dashboardLink(n), n.Status, defaultText(n.Next, "not recorded"))
	}
	if count == 0 {
		b.WriteString("No selected work recorded yet.\n")
	}
	b.WriteString("\n## Needs attention\n\n")
	count = 0
	for _, n := range notes {
		if !filter(n) || n.Kind == "run" || n.Kind == "event" || n.Kind == "checkpoint" || n.Status == "done" || n.Status == "archived" {
			continue
		}
		var reasons []string
		if n.Status == "active" && n.Next == "" {
			reasons = append(reasons, "missing next action")
		}
		if n.Status == "blocked" {
			reasons = append(reasons, "blocked")
		}
		if n.Domain == "unknown" {
			reasons = append(reasons, "work/home unassigned")
		}
		t, _ := time.Parse(time.RFC3339Nano, n.Updated)
		if n.Status == "active" && time.Since(t) > 7*24*time.Hour {
			reasons = append(reasons, "active state older than seven days")
		}
		if len(reasons) > 0 {
			count++
			fmt.Fprintf(&b, "- %s: %s.\n", s.dashboardLink(n), strings.Join(reasons, ", "))
		}
	}
	if count == 0 {
		b.WriteString("No flagged items in the recorded data.\n")
	}
	b.WriteString("\n## Execution observations\n\n")
	for _, n := range LatestRuns(notes) {
		if filter(n) {
			fmt.Fprintf(&b, "- %s — %s; observed %s.\n", s.dashboardLink(n), n.Status, n.Updated)
		}
	}
	b.WriteString("\n## Capture inbox\n\n")
	count = 0
	for _, n := range notes {
		if filter(n) && n.Kind != "project" && (n.Status == "inbox" || n.Status == "suggested") {
			count++
			fmt.Fprintf(&b, "- %s — %s\n", s.dashboardLink(n), n.Status)
			if count == 10 {
				b.WriteString("\nUse the Inbox view for all captures.\n")
				break
			}
		}
	}
	b.WriteString("\n## Measurements\n\n")
	totals := map[string]float64{}
	coverage := map[string]int{}
	for _, n := range LatestRuns(notes) {
		if !filter(n) {
			continue
		}
		for k, v := range n.Metrics {
			key := n.Source + " / " + k
			totals[key] += v
			coverage[key]++
		}
	}
	for _, k := range sortedMetricKeys(totals) {
		fmt.Fprintf(&b, "- %s: %.0f (reported by %d latest run snapshots).\n", k, totals[k], coverage[k])
	}
	b.WriteString("\nThese are recorded workflow outcomes, not a productivity score. Unreported time, tokens, cost, and human effort remain unknown.\n\n## Recorded recovery copies\n\n")
	for _, n := range notes {
		if filter(n) && n.Source == "thread-snapshot" {
			fmt.Fprintf(&b, "- %s — %s; working files only; may predate current changes.\n", s.dashboardLink(n), n.Created)
		}
	}
	b.WriteString("\nCurrent code protection remains unverified until compared with a recovery copy. Inspection alone never creates a backup.\n")
	return b.String()
}

func (s *Store) renderCategory(b *strings.Builder, notes []Note, filter func(Note) bool, kind string) {
	var category []Note
	for _, n := range notes {
		if filter(n) && n.Kind == kind {
			category = append(category, n)
		}
	}
	sort.Slice(category, func(i, j int) bool { return category[i].ID < category[j].ID })
	if len(category) == 0 {
		b.WriteString("No records yet.\n")
		return
	}
	for _, n := range category {
		fmt.Fprintf(b, "- %s\n", s.dashboardLink(n))
	}
}

type sessionEvidence struct {
	note     Note
	occurred time.Time
	date     string
	summary  string
}

func (s *Store) renderProjectSessions(b *strings.Builder, project Note, notes []Note, filter func(Note) bool) {
	evidence := projectSessionEvidence(project, notes, filter)
	if len(evidence) == 0 {
		b.WriteString("No session evidence recorded yet.\n\n")
		return
	}
	for i, item := range evidence {
		if i == 0 || evidence[i-1].date != item.date {
			fmt.Fprintf(b, "#### %s\n\n", item.date)
		}
		fmt.Fprintf(b, "- %s — %s\n", s.dashboardLink(item.note), item.summary)
	}
	b.WriteByte('\n')
}

func projectSessionEvidence(project Note, notes []Note, filter func(Note) bool) []sessionEvidence {
	var evidence []sessionEvidence
	for _, n := range notes {
		if !filter(n) || !SameLink(n.Project, project.PathLink()) {
			continue
		}
		if n.Kind == "session" {
			if occurred, err := time.Parse(time.RFC3339Nano, n.Created); err == nil {
				evidence = append(evidence, sessionEvidence{note: n, occurred: occurred, date: occurred.Format("2006-01-02"), summary: noteSummary(n)})
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
		evidence = append(evidence, sessionEvidence{note: n, occurred: occurred, date: occurred.Format("2006-01-02"), summary: summary})
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].occurred.Equal(evidence[j].occurred) {
			return evidence[i].note.ID < evidence[j].note.ID
		}
		return evidence[i].occurred.Before(evidence[j].occurred)
	})
	return evidence
}

// dashboardLink keeps generated, graph-facing navigation out of Thread's
// managed dot-directories. Record IDs are vault-wide unique, so Obsidian can
// resolve them by filename while projects retain their stable public path.
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
	if body := strings.TrimSpace(n.Body); body != "" {
		for _, line := range strings.Split(body, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return line
			}
		}
	}
	return n.Title
}
func sortedMetricKeys(m map[string]float64) []string {
	x := map[string]any{}
	for k := range m {
		x[k] = nil
	}
	return keys(x)
}
func defaultText(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// DashboardResult identifies the generated view and any intentionally skipped
// dataless vault records used to produce it.
type DashboardResult struct {
	Path    string
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
	body := s.renderOverview(notes, domain, warning)
	p, err := s.path("Thread/Overview.md")
	if err != nil {
		return DashboardResult{}, err
	}
	// This file is explicitly generated. User-owned notes are never rendered over.
	const marker = "<!-- thread-generated-overview -->\n"
	old, err := os.ReadFile(p)
	if err == nil && !strings.HasPrefix(string(old), marker) {
		return DashboardResult{}, fmt.Errorf("%s exists without the generated marker; refusing to overwrite", p)
	}
	if err != nil && !os.IsNotExist(err) {
		return DashboardResult{}, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return DashboardResult{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".thread-overview-*")
	if err != nil {
		return DashboardResult{}, err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(marker + body); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return DashboardResult{}, err
	}
	if ce != nil {
		return DashboardResult{}, ce
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return DashboardResult{}, err
	}
	if err := syncDir(filepath.Dir(p)); err != nil {
		return DashboardResult{}, err
	}
	return DashboardResult{Path: p, Warning: warning}, nil
}
