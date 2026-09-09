package thread

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitState struct {
	Root      string `json:"root"`
	Common    string `json:"common"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Changes   string `json:"changes"`
	Worktrees string `json:"worktrees"`
	Recovery  string `json:"recovery"`
}

func git(repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-C", repo}, args...)...)
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	for _, a := range args {
		if a == "-z" {
			return string(b), nil
		}
	}
	return strings.TrimSpace(string(b)), nil
}
func Inspect(repo string) (GitState, error) {
	var s GitState
	var err error
	s.Root, err = git(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return s, err
	}
	s.Common, err = git(repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return s, err
	}
	s.Common, _ = filepath.EvalSymlinks(s.Common)
	s.Head, err = git(repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		s.Head = "unborn"
	}
	s.Branch, err = git(repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		s.Branch = "detached"
	}
	s.Changes, err = git(repo, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return s, err
	}
	s.Worktrees, err = git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return s, err
	}
	s.Recovery = "unverified: repository inspection does not create a backup"
	return s, nil
}
func (s *Store) ResolveProject(id, repo string) (Note, error) {
	if id != "" {
		return s.Project(id)
	}
	state, err := Inspect(repo)
	if err != nil {
		return Note{}, err
	}
	notes, err := s.Notes()
	if err != nil {
		return Note{}, err
	}
	var matches []Note
	for _, n := range notes {
		if n.Kind != "project" || n.Repo == "" {
			continue
		}
		other, e := git(n.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			continue
		}
		other, _ = filepath.EvalSymlinks(other)
		if other == state.Common {
			matches = append(matches, n)
		}
	}
	if len(matches) > 1 {
		return Note{}, fmt.Errorf("found %d registered projects for this repository; pass --project ID or register it", len(matches))
	}
	if len(matches) == 0 {
		// Every session is worth recovering. Create a conservative project shell
		// instead of dropping an otherwise valid hook event. The repository's
		// common Git directory gives worktrees one durable identity.
		id := "project-auto-" + Hash([]byte(state.Common))[:32]
		n := NewNote("project", filepath.Base(state.Root))
		n.ID = id
		n.Repo = state.Root
		n.Domain = DefaultDomain(Machine())
		n.Status = "paused"
		n.Source = "auto-discovery"
		n.Extra = map[string]any{"git_common_dir": state.Common, "discovered_from": repo, "identity_status": "temporary", "suggested_name": filepath.Base(state.Root)}
		n.Body = "\nAutomatically discovered by a Thread lifecycle hook. Confirm the project name, domain, and ownership before treating it as active work.\n"
		path, createErr := s.New(n)
		if createErr != nil && !os.IsExist(createErr) {
			return Note{}, createErr
		}
		if createErr == nil {
			n.Path = path
			return n, nil
		}
		return s.Project(id)
	}
	return matches[0], nil
}
