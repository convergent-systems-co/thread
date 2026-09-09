package thread

import (
	"fmt"
	"os"
	"path/filepath"
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
		"Thread/Guide.md":      "# Using Thread\n\nCapture first; organize later. Edit item properties in Obsidian: `status`, `next`, `tags`, `related`, and `domain`. CLI updates preserve extra properties and body text.\n\nStatuses: inbox, suggested, active, paused, blocked, done, archived. Suggestions are not commitments. Set one concrete `next` action on active work.\n\nEach item links to its project. Use `related` links for dependencies and shared ideas; explain the relationship in the note body. The graph follows these links.\n\n`.items/` is Thread-managed storage for captures, sessions, run observations, and recovery records. Do not move, rename, or reorganize files there manually. Humans may correct properties when needed; use the CLI for lifecycle changes.\n\nRun observations are immutable snapshots from develop/Praxis. Thread does not advance their execution state. The generated Overview is refreshed by `thread dashboard`; human-facing work views live in Start Here, Overview, Thread.base, and project notes.\n\nCLI edits keep prior note versions in `Thread/.history`. This is local note history, not a separate backup or code protection. Avoid editing the same note simultaneously on multiple machines: iCloud is eventually consistent. Duplicate IDs and malformed notes are surfaced as errors.\n\nUse `thread snapshot --repo PATH` for a verified local working-file ZIP and `thread organize --id ID` for optional headless Claude classification. Automatic hooks, live sessions, and background polling are not enabled in this release. Unknown metrics are not zero, and session duration is not human working time.\n",
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
	var b strings.Builder
	fmt.Fprintf(&b, "# Thread overview\n\nGenerated %s. Source observations may be older.\n\n", Now())
	filter := func(n Note) bool { return domain == "" || n.Domain == domain }
	b.WriteString("## Work to resume\n\n")
	count := 0
	for _, n := range notes {
		if !filter(n) || n.Kind == "run" || n.Kind == "project" || n.Status == "done" || n.Status == "archived" || n.Status == "inbox" || n.Status == "suggested" {
			continue
		}
		count++
		fmt.Fprintf(&b, "- %s — %s; next: %s\n", s.Link(n), n.Status, defaultText(n.Next, "not recorded"))
	}
	if count == 0 {
		b.WriteString("No selected work recorded yet.\n")
	}
	b.WriteString("\n## Needs attention\n\n")
	count = 0
	for _, n := range notes {
		if !filter(n) || n.Kind == "run" || n.Status == "done" || n.Status == "archived" {
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
			fmt.Fprintf(&b, "- %s: %s.\n", s.Link(n), strings.Join(reasons, ", "))
		}
	}
	if count == 0 {
		b.WriteString("No flagged items in the recorded data.\n")
	}
	b.WriteString("\n## Execution observations\n\n")
	for _, n := range LatestRuns(notes) {
		if filter(n) {
			fmt.Fprintf(&b, "- %s — %s; observed %s.\n", s.Link(n), n.Status, n.Updated)
		}
	}
	b.WriteString("\n## Capture inbox\n\n")
	count = 0
	for _, n := range notes {
		if filter(n) && n.Kind != "project" && (n.Status == "inbox" || n.Status == "suggested") {
			count++
			fmt.Fprintf(&b, "- %s — %s\n", s.Link(n), n.Status)
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
			fmt.Fprintf(&b, "- %s — %s; working files only; may predate current changes.\n", s.Link(n), n.Created)
		}
	}
	b.WriteString("\nCurrent code protection remains unverified until compared with a recovery copy. Inspection alone never creates a backup.\n")
	return b.String(), nil
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
func (s *Store) Dashboard() (string, error) {
	body, err := s.Overview("")
	if err != nil {
		return "", err
	}
	p, err := s.path("Thread/Overview.md")
	if err != nil {
		return "", err
	}
	// This file is explicitly generated. User-owned notes are never rendered over.
	const marker = "<!-- thread-generated-overview -->\n"
	old, err := os.ReadFile(p)
	if err == nil && !strings.HasPrefix(string(old), marker) {
		return "", fmt.Errorf("%s exists without the generated marker; refusing to overwrite", p)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".thread-overview-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(marker + body); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return "", err
	}
	if ce != nil {
		return "", ce
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return "", err
	}
	return p, syncDir(filepath.Dir(p))
}
