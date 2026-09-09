package thread

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRecoveryContainsTrackedAndUntrackedBytes(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", repo}, {"-C", repo, "config", "user.email", "test@example.invalid"}, {"-C", repo, "config", "user.name", "Test"}} {
		if b, e := exec.Command("git", args...).CombinedOutput(); e != nil {
			t.Fatalf("%s %v", b, e)
		}
	}
	os.WriteFile(filepath.Join(repo, "main.go"), []byte("original"), 0600)
	exec.Command("git", "-C", repo, "add", "main.go").Run()
	exec.Command("git", "-C", repo, "commit", "-m", "initial").Run()
	os.WriteFile(filepath.Join(repo, "main.go"), []byte("valuable uncommitted code"), 0600)
	os.WriteFile(filepath.Join(repo, "new.go"), []byte("untracked idea"), 0600)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("secret.env\n"), 0600)
	os.WriteFile(filepath.Join(repo, "secret.env"), []byte("excluded"), 0600)
	before, _ := Inspect(repo)
	path := filepath.Join(t.TempDir(), "recovery.zip")
	if _, err := Snapshot(repo, path); err != nil {
		t.Fatal(err)
	}
	m, err := VerifySnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Files["main.go"] != Hash([]byte("valuable uncommitted code")) || m.Files["new.go"] == "" {
		t.Fatal("missing work")
	}
	if m.Files["secret.env"] != "" {
		t.Fatal("included ignored file")
	}
	after, _ := Inspect(repo)
	if before.Head != after.Head || before.Changes != after.Changes {
		t.Fatal("changed checkout")
	}
	z, _ := zip.OpenReader(path)
	defer z.Close()
	for _, f := range z.File {
		if f.Name == "files/new.go" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			if string(b) != "untracked idea" {
				t.Fatal("cannot recover content")
			}
		}
	}
	if _, err = Snapshot(repo, path); err == nil {
		t.Fatal("overwrote existing archive")
	}
}
