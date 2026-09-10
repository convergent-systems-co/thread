package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Cues deliberately omit bodies and opaque provider data. Sources remain
// available through show or Obsidian rather than filling reasoning context.
type Cue struct {
	ID       string `json:"id"`
	Link     string `json:"link"`
	Kind     string `json:"kind"`
	Status   string `json:"status,omitempty"`
	Text     string `json:"text"`
	Next     string `json:"next,omitempty"`
	Observed string `json:"observed"`
}

type AttentionCue struct {
	Cue    Cue    `json:"source"`
	Reason string `json:"reason"`
}

type Attention struct {
	NeedsAttention []AttentionCue `json:"needs_attention"`
	CanSetDown     []AttentionCue `json:"can_set_down"`
}

type SessionObservation struct {
	Source    string `json:"source"`
	SessionID string `json:"session_id"`
	Machine   string `json:"machine,omitempty"`
	State     string `json:"last_observed_state"`
	Cue       Cue    `json:"source_event"`
}

type Orientation struct {
	Project      Cue                  `json:"project"`
	Work         []Cue                `json:"work"`
	Reference    []Cue                `json:"reference"`
	Suggestions  []Cue                `json:"suggestions"`
	Observations []Cue                `json:"observations"`
	Sessions     []SessionObservation `json:"sessions"`
	Attention    Attention            `json:"attention"`
	Omitted      map[string]int       `json:"omitted"`
}

func excerpt(text string) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) > 240 {
		return string(r[:237]) + "..."
	}
	return string(r)
}

func (s *Store) cue(n Note) Cue {
	return Cue{ID: n.ID, Link: s.Link(n), Kind: n.Kind, Status: n.Status,
		Text: excerpt(n.Title), Next: excerpt(n.Next), Observed: n.Updated}
}

// Orient derives a bounded view from the vault. It does not infer commitments,
// require the original machine, or treat a lifecycle event as a live lease.
func (s *Store) Orient(projectID string, limit int) (Orientation, error) {
	var result Orientation
	if limit < 1 || limit > 20 {
		return result, errors.New("context limit must be between 1 and 20")
	}
	project, err := s.Project(projectID)
	if err != nil {
		return result, err
	}
	notes, err := s.Notes()
	if err != nil {
		return result, err
	}
	result = Orientation{Project: s.cue(project), Work: []Cue{}, Reference: []Cue{},
		Suggestions: []Cue{}, Observations: []Cue{}, Sessions: []SessionObservation{},
		Attention: Attention{NeedsAttention: []AttentionCue{}, CanSetDown: []AttentionCue{}},
		Omitted: map[string]int{}}
	add := func(section string, list *[]Cue, cue Cue) {
		if len(*list) < limit {
			*list = append(*list, cue)
		} else {
			result.Omitted[section]++
		}
	}
	attention := func(needs bool, cue Cue, reason string) {
		section, list := "can_set_down", &result.Attention.CanSetDown
		if needs {
			section, list = "needs_attention", &result.Attention.NeedsAttention
		}
		if len(*list) < limit {
			*list = append(*list, AttentionCue{Cue: cue, Reason: reason})
		} else {
			result.Omitted[section]++
		}
	}
	// Stable ordering makes equal-timestamp captures and checkpoint hashes
	// deterministic, including events delivered out of order.
	timestamp := func(n Note) time.Time {
		value := n.Updated
		if n.Kind == "event" {
			value, _ = n.Extra["occurred_at"].(string)
		}
		t, _ := time.Parse(time.RFC3339Nano, value)
		return t
	}
	sort.Slice(notes, func(i, j int) bool {
		a, b := timestamp(notes[i]), timestamp(notes[j])
		if a.Equal(b) {
			return notes[i].ID < notes[j].ID
		}
		return a.After(b)
	})
	seenSessions := map[string]bool{}
	for _, n := range notes {
		if !SameLink(n.Project, s.Link(project)) || n.Kind == "checkpoint" || n.Kind == "run" {
			continue
		}
		cue := s.cue(n)
		if n.Kind == "event" {
			e, err := noteEvent(n)
			if err != nil {
				return result, err
			}
			cue.Kind, cue.Observed = e.Type, e.OccurredAt
			cue.Text = e.Type
			if e.Text != "" {
				cue.Text += ": " + excerpt(e.Text)
			}
			if strings.HasPrefix(e.Type, "session.") {
				key, _ := json.Marshal([]string{e.Source, e.Machine, e.SessionID})
				if !seenSessions[string(key)] {
					seenSessions[string(key)] = true
					state := strings.TrimPrefix(e.Type, "session.")
					if len(result.Sessions) < limit {
						result.Sessions = append(result.Sessions, SessionObservation{Source: e.Source, SessionID: e.SessionID, Machine: e.Machine, State: state, Cue: cue})
					} else {
						result.Omitted["sessions"]++
					}
					if state == "started" || state == "resumed" {
						attention(true, cue, "No later idle/stop recorded; current activity and stopping point are unknown.")
					} else {
						attention(false, cue, "Lifecycle boundary is saved; this does not establish work completion or code recovery.")
					}
				}
			}
			if strings.HasPrefix(e.Type, "prompt.") || strings.HasPrefix(e.Type, "tool.") {
				result.Omitted["low_level_events"]++
				continue
			}
			add("observations", &result.Observations, cue)
			continue
		}
		switch n.Status {
		case "suggested":
			add("suggestions", &result.Suggestions, cue)
			attention(false, cue, "Saved suggestion, not an accepted commitment.")
			continue
		case "done", "archived":
			attention(false, cue, "Recorded as "+n.Status+"; no active commitment in this record. Not independent proof of completion.")
			continue
		case "blocked":
			attention(true, cue, "Recorded blocker remains unresolved.")
		case "active":
			if strings.TrimSpace(n.Next) == "" {
				attention(true, cue, "Active record has no return cue; the next step is unknown.")
			}
		case "paused":
			if strings.TrimSpace(n.Next) != "" {
				attention(false, cue, "Paused with a saved return cue; no reminder or background monitoring is scheduled.")
			} else {
				attention(true, cue, "Paused without a return cue; the next step is unknown.")
			}
		case "inbox":
			attention(false, cue, "Original wording is saved for later; not scheduled or accepted as work.")
		}
		if n.Status == "active" || n.Status == "paused" || n.Status == "blocked" {
			add("work", &result.Work, cue)
		} else {
			add("reference", &result.Reference, cue)
		}
	}
	if len(result.Work) == 0 && strings.TrimSpace(project.Next) != "" {
		add("work", &result.Work, result.Project)
	}
	return result, nil
}

func RenderOrientation(o Orientation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nSource: %s\n\n", o.Project.Text, o.Project.Link)
	for _, section := range []struct {
		name string
		cues []Cue
	}{{"Return cues", o.Work}, {"Reference", o.Reference}, {"Suggestions (not commitments)", o.Suggestions}, {"Recent observations (not verified outcomes)", o.Observations}} {
		fmt.Fprintf(&b, "## %s\n\n", section.name)
		for _, cue := range section.cues {
			fmt.Fprintf(&b, "- %s [%s] %s", cue.Text, cue.Status, cue.Link)
			if cue.Next != "" {
				fmt.Fprintf(&b, "; next: %s", cue.Next)
			}
			fmt.Fprintf(&b, " (observed %s)\n", cue.Observed)
		}
		if len(section.cues) == 0 {
			b.WriteString("Not recorded.\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Last observed sessions (not live activity)\n\n")
	for _, session := range o.Sessions {
		fmt.Fprintf(&b, "- %s / %s: %s, %s %s\n", excerpt(session.Source), excerpt(session.SessionID), session.State, session.Cue.Observed, session.Cue.Link)
	}
	b.WriteString("\n## Needs attention\n\n")
	for _, a := range o.Attention.NeedsAttention {
		fmt.Fprintf(&b, "- %s %s: %s\n", a.Cue.Text, a.Cue.Link, a.Reason)
	}
	if len(o.Attention.NeedsAttention) == 0 {
		b.WriteString("No flags in the recorded context; unrecorded obligations remain unknown.\n")
	}
	b.WriteString("\n## Can stop holding in working memory\n\n")
	for _, a := range o.Attention.CanSetDown {
		fmt.Fprintf(&b, "- %s %s: %s\n", a.Cue.Text, a.Cue.Link, a.Reason)
	}
	if len(o.Attention.CanSetDown) == 0 {
		b.WriteString("No supported set-down cues recorded yet.\n")
	}
	if len(o.Omitted) != 0 {
		b.WriteString("\nOmitted from this brief:")
		for _, key := range keysInt(o.Omitted) {
			fmt.Fprintf(&b, " %s=%d", key, o.Omitted[key])
		}
		b.WriteString(".\n")
	}
	b.WriteString("\nUse thread show --id ID or follow a source link for original wording. This is recorded context, not a backup or a live monitor.\n")
	return b.String()
}

func keysInt(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Checkpoint saves a compact, source-linked view, not a transcript. Identical
// views deduplicate; originals are never deleted or rewritten.
func (s *Store) Checkpoint(projectID string) (string, error) {
	o, err := s.Orient(projectID, 5)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	n := NewNote("checkpoint", "Operational checkpoint: "+o.Project.Text)
	n.ID = "checkpoint-" + Hash(raw)
	n.Source, n.Status = "thread-consolidation", ""
	n.Project = o.Project.Link
	project, err := s.Project(projectID)
	if err != nil {
		return "", err
	}
	n.Domain = project.Domain
	n.Extra = map[string]any{"context_json": string(raw), "consolidation": "deterministic-v1"}
	n.Body = "\nHistorical checkpoint; later observations may supersede it.\n\n" + RenderOrientation(o)
	n.Related = []string{o.Project.Link}
	path, err := s.New(n)
	if !os.IsExist(err) {
		return path, err
	}
	existing, err := s.Find(n.ID)
	if err != nil {
		return "", err
	}
	if existing.Kind != "checkpoint" || existing.Extra["context_json"] != string(raw) {
		return "", errors.New("checkpoint identity conflict; original retained")
	}
	return existing.Path, nil
}
