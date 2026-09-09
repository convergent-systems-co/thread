package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCaptureWithoutAIAndResumeRepository(t *testing.T) {
	vault := t.TempDir()
	repo := t.TempDir()
	cmd := exec.Command("git", "init", repo)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v", b, err)
	}
	var out bytes.Buffer
	call := func(args ...string) {
		t.Helper()
		out.Reset()
		if err := run(append([]string{"--vault", vault}, args...), strings.NewReader(""), &out); err != nil {
			t.Fatal(err)
		}
	}
	call("init")
	call("project", "--id", "demo", "--repo", repo, "--domain", "home", "Demo")
	call("capture", "--project", "demo", "--kind", "action", "--status", "active", "--next", "Ask Dana", "Conference call follow-up")
	call("resume", "--repo", repo)
	if !strings.Contains(out.String(), "Ask Dana") || !strings.Contains(out.String(), "unverified") {
		t.Fatal(out.String())
	}
	call("status")
	if !strings.Contains(out.String(), "Conference call follow-up") {
		t.Fatal(out.String())
	}
}
