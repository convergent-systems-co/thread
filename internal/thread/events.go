package thread

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxEventBytes = 4 * 1024 * 1024
const eventFolder = ".events"

// Event is Thread's provider-neutral capture boundary. Data is optional
// producer-specific evidence, not instructions or an inferred work state.
type Event struct {
	Schema     int            `json:"schema"`
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Source     string         `json:"source"`
	SessionID  string         `json:"session_id,omitempty"`
	OccurredAt string         `json:"occurred_at"`
	ProjectID  string         `json:"project_id,omitempty"`
	Repo       string         `json:"repo,omitempty"`
	Machine    string         `json:"machine,omitempty"`
	Text       string         `json:"text,omitempty"`
	References []string       `json:"references,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

var eventTypes = map[string]bool{
	"session.started": true, "session.stopped": true, "session.idle": true,
	"session.resumed": true, "prompt.started": true, "prompt.completed": true,
	"tool.before": true, "tool.after": true, "artifact.created": true,
	"artifact.modified": true, "decision.observed": true, "action.observed": true,
	"context.changed": true, "project.changed": true, "checkpoint.requested": true,
}

func DecodeEvent(data []byte) (Event, error) {
	var e Event
	if len(data) > MaxEventBytes {
		return e, errors.New("event payload exceeds 4 MiB")
	}
	if !utf8.Valid(data) {
		return e, errors.New("event payload is not valid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&e); err != nil {
		return e, fmt.Errorf("invalid event JSON: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return e, errors.New("event input must contain exactly one JSON object")
	}
	return e, e.validate()
}

func (e Event) validate() error {
	if e.Schema != 1 || !eventTypes[e.Type] {
		return errors.New("event requires schema 1 and a supported type")
	}
	if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Source) == "" || len(e.ID) > 512 || len(e.Source) > 200 {
		return errors.New("event requires id (up to 512 bytes) and source (up to 200 bytes)")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.OccurredAt); err != nil {
		return fmt.Errorf("event requires an RFC3339 occurred_at: %w", err)
	}
	if (strings.HasPrefix(e.Type, "session.") || strings.HasPrefix(e.Type, "prompt.") || strings.HasPrefix(e.Type, "tool.")) && strings.TrimSpace(e.SessionID) == "" {
		return errors.New("session, prompt and tool events require session_id")
	}
	if e.ProjectID != "" && !safeID.MatchString(e.ProjectID) {
		return errors.New("invalid event project_id")
	}
	return nil
}

func eventNoteID(source, id string) string {
	key, _ := json.Marshal([]string{source, id})
	return "event-" + Hash(key)
}

// CaptureEvent acknowledges only durable capture. It does not run AI, read a
// transcript, inspect a repository, or require a project to be registered.
func (s *Store) CaptureEvent(e Event) (string, error) {
	return s.captureEvent(e, false)
}

func (s *Store) captureEvent(e Event, receiptTime bool) (string, error) {
	if receiptTime {
		e.OccurredAt = Now()
	}
	if err := e.validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("encode event: %w", err)
	}
	if len(raw) > MaxEventBytes {
		return "", errors.New("event payload exceeds 4 MiB")
	}
	n := NewNote("event", e.Type+": "+e.Source)
	n.ID = eventNoteID(e.Source, e.ID)
	n.Status = ""
	// A receiving machine's default domain is not evidence of event ownership.
	n.Domain = "unknown"
	n.Source = e.Source
	n.Repo = e.Repo
	if e.Machine != "" {
		n.Machine = e.Machine
	}
	// Linking by stable ID allows offline capture before project registration.
	if e.ProjectID != "" {
		n.Project = "[[Thread/Projects/" + e.ProjectID + "]]"
	}
	n.Extra = map[string]any{"event_json": string(raw), "event_type": e.Type, "occurred_at": e.OccurredAt, "session_id": e.SessionID}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "    ", "  "); err != nil {
		return "", err
	}
	n.Body = "\nCaptured observation, not a commitment or proof of completed work.\n\n## Original event\n\n    " + pretty.String() + "\n"
	path, err := s.New(n)
	if !os.IsExist(err) {
		return path, err
	}
	existing, err := s.Find(n.ID)
	if err != nil {
		return "", err
	}
	if receiptTime && existing.Kind == "event" {
		previous, err := noteEvent(existing)
		if err != nil {
			return "", err
		}
		e.OccurredAt = previous.OccurredAt
		raw, err = json.Marshal(e)
		if err != nil {
			return "", err
		}
	}
	if existing.Kind != "event" || existing.Extra["event_json"] != string(raw) {
		return "", fmt.Errorf("event identity conflict for source %q id %q; original retained", e.Source, e.ID)
	}
	return existing.Path, nil
}

func noteEvent(n Note) (Event, error) {
	raw, ok := n.Extra["event_json"].(string)
	if !ok {
		return Event{}, fmt.Errorf("event %s has no original event_json", n.ID)
	}
	e, err := DecodeEvent([]byte(raw))
	if err != nil {
		return e, fmt.Errorf("event %s: %w", n.ID, err)
	}
	if eventNoteID(e.Source, e.ID) != n.ID {
		return e, fmt.Errorf("event %s identity differs from original event", n.ID)
	}
	return e, nil
}
