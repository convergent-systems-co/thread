package thread

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractRepositoriesCreatesPersonalMachineRecordsWithoutTouchingRepos(t *testing.T) {
	s := testStore(t)
	root := t.TempDir()
	repo := filepath.Join(root, "private-one")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	before, _ := Inspect(repo)
	notes, err := s.ExtractRepositories(root, "heimdall", "home")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Domain != "home" || notes[0].Machine != "heimdall" || notes[0].Source != "machine-inventory" {
		t.Fatalf("%+v", notes)
	}
	after, _ := Inspect(repo)
	if before.Head != after.Head || before.Changes != after.Changes {
		t.Fatal("inventory changed repository")
	}
	again, err := s.ExtractRepositories(root, "heimdall", "home")
	if err != nil || len(again) != 1 || again[0].ID != notes[0].ID {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
}
