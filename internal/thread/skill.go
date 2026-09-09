package thread

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/thread/SKILL.md assets/thread/agents/openai.yaml
var embeddedSkill embed.FS

var skillFiles = []string{"SKILL.md", "agents/openai.yaml"}

func InstallSkill(client string, force bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var roots []string
	switch client {
	case "codex":
		roots = []string{filepath.Join(home, ".codex", "skills", "thread")}
	case "claude":
		roots = []string{filepath.Join(home, ".claude", "skills", "thread")}
	case "all":
		roots = []string{filepath.Join(home, ".codex", "skills", "thread"), filepath.Join(home, ".claude", "skills", "thread")}
	default:
		return nil, errors.New("client must be codex, claude, or all")
	}
	var installed []string
	for _, root := range roots {
		for _, rel := range skillFiles {
			data, e := embeddedSkill.ReadFile("assets/thread/" + rel)
			if e != nil {
				return installed, e
			}
			dest := filepath.Join(root, rel)
			old, e := os.ReadFile(dest)
			if e == nil && !bytes.Equal(old, data) && !force {
				return installed, fmt.Errorf("skill file exists with different content: %s (use --force to replace)", dest)
			}
			if e == nil && bytes.Equal(old, data) {
				installed = append(installed, dest)
				continue
			}
			if e != nil && !os.IsNotExist(e) {
				return installed, e
			}
			if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
				return installed, e
			}
			if e = os.WriteFile(dest, data, 0600); e != nil {
				return installed, e
			}
			installed = append(installed, dest)
		}
	}
	return installed, nil
}
