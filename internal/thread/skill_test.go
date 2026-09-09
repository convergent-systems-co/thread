package thread

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkillRefusesChangedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths, err := InstallSkill("codex", false)
	if err != nil || len(paths) != 2 {
		t.Fatalf("%v %v", paths, err)
	}
	p := filepath.Join(home, ".codex", "skills", "thread", "SKILL.md")
	if err := os.WriteFile(p, []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = InstallSkill("codex", false); err == nil {
		t.Fatal("overwrote custom skill")
	}
	if _, err = InstallSkill("codex", true); err != nil {
		t.Fatal(err)
	}
}
