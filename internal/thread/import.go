package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Each changed source produces a new immutable observation, never an overwrite
// of a human-edited record. source_key identifies the run across observations.
func (s *Store) ImportRun(provider, path, project string) (string, error) {
	p, err := s.Project(project)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if len(b) > 16*1024*1024 {
		return "", errors.New("state file exceeds 16 MiB")
	}
	var raw map[string]any
	if err = json.Unmarshal(b, &raw); err != nil {
		return "", err
	}
	runID := str(raw["run_id"])
	if runID == "" {
		return "", errors.New("missing run_id")
	}
	n := NewNote("run", provider+": "+runID)
	n.ID = "run-" + Hash([]byte(provider + "\x00" + project + "\x00" + abs + "\x00" + Hash(b)))[:40]
	n.Project = s.Link(p)
	n.Domain = p.Domain
	n.Source = provider
	n.Status = "paused"
	n.Repo = p.Repo
	n.Extra = map[string]any{"source_path": abs, "source_hash": Hash(b), "source_key": provider + ":" + project + ":" + abs, "run_id": runID}
	n.Metrics = map[string]float64{}
	var lines []string
	switch provider {
	case "develop":
		if str(raw["node"]) == "" {
			return "", errors.New("not a develop state: missing node")
		}
		state := str(raw["status"])
		n.Status = mapStatus(state)
		n.Extra["source_status"] = state
		n.Extra["source_updated"] = str(raw["updated_at"])
		lines = append(lines, "Orchestrator: `"+str(raw["node"])+"` ("+state+").")
		if x, ok := raw["handoffs"].(float64); ok {
			n.Metrics["handoffs"] = x
		}
		if x, ok := raw["repair_cycles"].(map[string]any); ok {
			for _, v := range x {
				if f, ok := v.(float64); ok {
					n.Metrics["repair_cycles"] += f
				}
			}
		}
		if tasks, present := raw["tasks_runtime"].(map[string]any); present {
			n.Metrics["recorded_tasks"] = float64(len(tasks))
			n.Metrics["completed_tasks"] = 0
			for _, v := range tasks {
				m, _ := v.(map[string]any)
				if str(m["status"]) == "complete" {
					n.Metrics["completed_tasks"]++
				}
			}
		}
		bundles, _ := raw["bundles_runtime"].(map[string]any)
		for _, key := range keys(bundles) {
			m, _ := bundles[key].(map[string]any)
			line := fmt.Sprintf("- %s: %s at %s", key, str(m["status"]), str(m["node"]))
			if wt := str(m["worktree"]); wt != "" {
				line += "; worktree: `" + wt + "`"
			}
			lines = append(lines, line)
		}
		if h := raw["human_interrupt"]; h != nil {
			j, _ := json.MarshalIndent(h, "", "  ")
			lines = append(lines, "\nRecorded human interrupt:\n\n```json\n"+string(j)+"\n```")
		}
		if c := raw["concerns"]; c != nil {
			j, _ := json.MarshalIndent(c, "", "  ")
			if string(j) != "[]" {
				lines = append(lines, "\nRecorded concerns:\n\n```json\n"+string(j)+"\n```")
			}
		}
		handoff := filepath.Join(filepath.Dir(abs), "HANDOFF.md")
		if _, err := os.Stat(handoff); err == nil {
			n.Extra["handoff_path"] = handoff
			lines = append(lines, "\nHandoff available at `"+handoff+"`.")
		}
	case "praxis":
		cursors, ok := raw["cursors"].(map[string]any)
		if !ok || str(raw["spec_version"]) == "" {
			return "", errors.New("not a Praxis state: missing cursors/spec_version")
		}
		n.Status = "done"
		if len(cursors) == 0 {
			n.Status = "paused"
		}
		n.Metrics["recorded_nodes"] = float64(len(cursors))
		n.Metrics["successful_nodes"] = 0
		for _, key := range keys(cursors) {
			m, _ := cursors[key].(map[string]any)
			state := str(m["status"])
			lines = append(lines, fmt.Sprintf("- %s: %s", key, state))
			if state == "terminal_success" {
				n.Metrics["successful_nodes"]++
			} else if state == "blocked" || state == "terminal_failed" || state == "handoff" {
				n.Status = "blocked"
			} else if n.Status != "blocked" {
				n.Status = "active"
			}
		}
	default:
		return "", errors.New("provider must be develop or praxis")
	}
	n.Body = "\n# " + n.Title + "\n\nObserved from `" + abs + "`. This is a source snapshot, not permission to resume execution.\n\n" + strings.Join(lines, "\n") + "\n"
	dest, err := s.New(n)
	if os.IsExist(err) {
		existing, e := s.Find(n.ID)
		if e != nil {
			return "", e
		}
		return existing.Path, nil
	}
	return dest, err
}
func str(v any) string { s, _ := v.(string); return s }
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func mapStatus(s string) string {
	switch s {
	case "running", "active":
		return "active"
	case "complete":
		return "done"
	case "human_required", "waiting_human", "blocked":
		return "blocked"
	default:
		return "paused"
	}
}

// LatestRuns selects one observation per run; polling never inflates totals.
func LatestRuns(notes []Note) []Note {
	seen := map[string]bool{}
	var out []Note
	for _, n := range notes {
		if n.Kind != "run" {
			continue
		}
		key := str(n.Extra["source_key"])
		if key == "" {
			key = n.ID
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, n)
		}
	}
	return out
}
