package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

func TestInitAndDashboardReportGeneratedPath(t *testing.T) {
	vault := t.TempDir()
	resolvedVault, err := filepath.EvalSymlinks(vault)
	if err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(resolvedVault, "Thread", "Overview.md")
	var out bytes.Buffer

	if err := run([]string{"--vault", vault, "init"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != generated+"\n" {
		t.Fatalf("init output = %q, want generated path %q", got, generated+"\n")
	}

	out.Reset()
	if err := run([]string{"--vault", vault, "dashboard"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Updated Thread dashboard: "+generated+"\n"; got != want {
		t.Fatalf("dashboard output = %q, want %q", got, want)
	}
}

func TestDashboardReportsPartialVaultWarning(t *testing.T) {
	vault := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"--vault", vault, "init"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(vault, "Thread", ".items", "evicted.md")
	if err := os.MkdirAll(filepath.Dir(missing), 0700); err != nil {
		t.Fatal(err)
	}
	// A nonempty sparse file has zero allocated blocks on APFS, matching an
	// iCloud FileProvider placeholder without depending on a real iCloud vault.
	f, err := os.OpenFile(missing, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(missing, 1); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := run([]string{"--vault", vault, "dashboard"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	resolvedVault, err := filepath.EvalSymlinks(vault)
	if err != nil {
		t.Fatal(err)
	}
	want := "Updated Thread dashboard: " + filepath.Join(resolvedVault, "Thread", "Overview.md") + "\n" +
		"Warning: 1 vault record is not downloaded; skipped dataless iCloud file Thread/.items/evicted.md. Results below may be incomplete.\n"
	if got := out.String(); got != want {
		t.Fatalf("partial dashboard output = %q, want %q", got, want)
	}
}

func TestDashboardPreservesRefreshFailure(t *testing.T) {
	vault := t.TempDir()
	overview := filepath.Join(vault, "Thread", "Overview.md")
	if err := os.MkdirAll(filepath.Dir(overview), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overview, []byte("user-owned overview"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := run([]string{"--vault", vault, "dashboard"}, strings.NewReader(""), &out)
	if err == nil || !strings.Contains(err.Error(), "exists without the generated marker") {
		t.Fatalf("dashboard error = %v, want generated-overview ownership failure", err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("dashboard reported success despite refresh failure: %q", got)
	}
}
