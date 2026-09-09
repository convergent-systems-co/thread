package thread

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExtractRepositories inventories repositories already present under root. It
// never clones, moves, edits, or deletes source projects.
func (s *Store) ExtractRepositories(root, machine, domain string) ([]Note, error) {
	if machine == "" {
		return nil, errors.New("machine is required")
	}
	if domain != "home" && domain != "work" && domain != "unknown" {
		return nil, errors.New("domain must be home, work, or unknown")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, errors.New("inventory root is not a directory")
	}
	existing, err := s.Notes()
	if err != nil {
		return nil, err
	}
	byCommon := map[string]Note{}
	for _, n := range existing {
		if n.Kind != "project" || n.Repo == "" {
			continue
		}
		if c, e := git(n.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir"); e == nil {
			c, _ = filepath.EvalSymlinks(c)
			byCommon[c] = n
		}
	}
	var found []Note
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(abs, path)
		depth := 0
		if rel != "." {
			depth = len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
		}
		if depth > 3 {
			return filepath.SkipDir
		}
		if path != abs {
			if _, e := os.Stat(filepath.Join(path, ".git")); e == nil {
				state, e := Inspect(path)
				if e != nil {
					return filepath.SkipDir
				}
				if old, ok := byCommon[state.Common]; ok {
					found = append(found, old)
					return filepath.SkipDir
				}
				base := filepath.Base(state.Root)
				id := "project-" + Hash([]byte(machine + "\x00" + state.Common))[:32]
				n := NewNote("project", base)
				n.ID = id
				n.Domain = domain
				n.Machine = machine
				n.Repo = state.Root
				n.Source = "machine-inventory"
				n.Status = "paused"
				n.Extra = map[string]any{"inventory_root": abs, "git_common_dir": state.Common, "inventory_machine": machine}
				n.Body = "\nDiscovered by Thread inventory on machine `" + machine + "`.\n\nRepository was not moved or cloned. Confirm ownership and project identity before adding work.\n"
				p, e := s.New(n)
				if e != nil && !os.IsExist(e) {
					return e
				}
				if e == nil {
					n.Path = p
				}
				found = append(found, n)
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

func InventorySummary(notes []Note) string {
	if len(notes) == 0 {
		return "No new repositories found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Registered %d repositories:\n", len(notes))
	for _, n := range notes {
		fmt.Fprintf(&b, "- %s · %s · %s\n", n.Title, n.Domain, n.Repo)
	}
	return b.String()
}
