package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	core "github.com/convergent-systems-co/thread/internal/thread"
)

func TestEventContextCheckpointCLI(t *testing.T) {
	vault := t.TempDir()
	s, err := core.Open(vault)
	if err != nil {
		t.Fatal(err)
	}
	p := core.NewNote("project", "Portable memory")
	p.ID = "portable"
	p.Repo = "/missing/on/this/machine"
	if _, err := s.New(p); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	event := `{"schema":1,"id":"stop-1","source":"any-editor","type":"session.stopped","session_id":"s1","occurred_at":"2026-09-09T12:00:00Z","project_id":"portable","text":"Investigating cache invalidation"}`
	if err := run([]string{"--vault", vault, "event"}, strings.NewReader(event), &out); err != nil {
		t.Fatal(err)
	}
	path := out.String()
	out.Reset()
	if err := run([]string{"--vault", vault, "event"}, strings.NewReader(event), &out); err != nil || out.String() != path {
		t.Fatalf("retry: %s %v", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"--vault", vault, "context", "--project", p.ID, "--json"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	var o core.Orientation
	if err := json.Unmarshal(out.Bytes(), &o); err != nil || len(o.Sessions) != 1 || o.Sessions[0].State != "stopped" {
		t.Fatalf("context: %s, %v", out.String(), err)
	}
	out.Reset()
	if err := run([]string{"--vault", vault, "checkpoint", "--project", p.ID}, strings.NewReader(""), &out); err != nil || !strings.Contains(out.String(), "checkpoint-") {
		t.Fatalf("checkpoint: %s, %v", out.String(), err)
	}
	if err := run([]string{"--vault", vault, "capture", "--kind", "event", "fake"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("bypassed event validation")
	}
}
