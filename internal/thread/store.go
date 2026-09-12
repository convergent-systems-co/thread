package thread

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Note properties remain editable in Obsidian. Unknown properties and body text
// survive CLI edits. Imported observations are separate from user-owned items.
type Note struct {
	Schema  int                `yaml:"thread_schema" json:"thread_schema"`
	ID      string             `yaml:"id" json:"id"`
	Kind    string             `yaml:"kind" json:"kind"`
	Title   string             `yaml:"title" json:"title"`
	Aliases []string           `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Project string             `yaml:"project,omitempty" json:"project,omitempty"`
	Domain  string             `yaml:"domain" json:"domain"`
	Status  string             `yaml:"status,omitempty" json:"status,omitempty"`
	Next    string             `yaml:"next,omitempty" json:"next,omitempty"`
	Source  string             `yaml:"source" json:"source"`
	Created string             `yaml:"created" json:"created"`
	Updated string             `yaml:"updated" json:"updated"`
	Machine string             `yaml:"machine" json:"machine"`
	Repo    string             `yaml:"repo,omitempty" json:"repo,omitempty"`
	Branch  string             `yaml:"branch,omitempty" json:"branch,omitempty"`
	Head    string             `yaml:"head,omitempty" json:"head,omitempty"`
	Tags    []string           `yaml:"tags,omitempty" json:"tags,omitempty"`
	Related []string           `yaml:"related,omitempty" json:"related,omitempty"`
	Metrics map[string]float64 `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Extra   map[string]any     `yaml:",inline" json:"extra,omitempty"`
	Body    string             `yaml:"-" json:"body"`
	Path    string             `yaml:"-" json:"path"`
}

type Store struct{ Root string }

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)
var kinds = map[string]bool{"project": true, "capture": true, "action": true, "decision": true, "discovery": true, "habit": true, "memory": true, "session": true, "run": true, "event": true, "checkpoint": true, "assumption": true, "question": true, "constraint": true, "artifact": true, "dependency": true, "risk": true, "direction": true, "outcome": true}
var statuses = map[string]bool{"inbox": true, "suggested": true, "active": true, "paused": true, "blocked": true, "done": true, "archived": true}

const itemFolder = ".items"
const runFolder = ".runs"

var vaultReadTimeout = 5 * time.Second
var readVaultFile = os.ReadFile
var datalessVaultFile = isDatalessFile

// VaultReadWarning makes an intentionally incomplete view visible to callers.
// Paths are bounded so a heavily evicted vault does not produce unbounded output.
type VaultReadWarning struct {
	Skipped int      `json:"skipped"`
	Paths   []string `json:"paths,omitempty"`
}

func (w VaultReadWarning) Error() string {
	if w.Skipped == 1 {
		return "1 vault record is not downloaded; skipped dataless iCloud file " + strings.Join(w.Paths, ", ")
	}
	return fmt.Sprintf("%d vault records are not downloaded; skipped dataless iCloud files (examples: %s)", w.Skipped, strings.Join(w.Paths, ", "))
}

type VaultReadError struct {
	Path string
	Err  error
}

func (e *VaultReadError) Error() string {
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return fmt.Sprintf("vault read timed out after %s while reading %s; synced content may not be downloaded", vaultReadTimeout, e.Path)
	}
	return fmt.Sprintf("vault record unavailable at %s: %v", e.Path, e.Err)
}

func (e *VaultReadError) Unwrap() error { return e.Err }

func IsVaultUnavailable(err error) bool {
	var readErr *VaultReadError
	var warning VaultReadWarning
	return errors.As(err, &readErr) || errors.As(err, &warning)
}

func Now() string     { return time.Now().UTC().Format(time.RFC3339Nano) }
func Machine() string { h, _ := os.Hostname(); return h }
func DefaultDomain(machine string) string {
	switch strings.ToLower(strings.Split(machine, ".")[0]) {
	case "odin":
		return "home"
	case "hindal":
		return "work"
	}
	return "unknown"
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func NewNote(kind, title string) Note {
	t := Now()
	return Note{Schema: 1, ID: ID(), Kind: kind, Title: title, Aliases: []string{title}, Domain: DefaultDomain(Machine()), Status: "inbox", Source: "manual", Created: t, Updated: t, Machine: Machine(), Tags: []string{"thread", "thread/" + kind}}
}
func Open(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("set THREAD_VAULT or pass --vault PATH before the command")
	}
	p, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return nil, fmt.Errorf("vault must already exist: %w", err)
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, errors.New("vault is not a directory")
	}
	return &Store{Root: p}, nil
}
func (s *Store) path(rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute vault-relative path")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes vault")
	}
	p := s.Root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		st, err := os.Lstat(p)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlink in Thread storage: %s", p)
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	return p, nil
}
func validate(n Note) error {
	if n.Schema != 1 || !safeID.MatchString(n.ID) || !kinds[n.Kind] || strings.TrimSpace(n.Title) == "" {
		return errors.New("invalid Thread schema, id, kind, or title")
	}
	if n.Domain != "home" && n.Domain != "work" && n.Domain != "unknown" {
		return errors.New("domain must be home, work, or unknown")
	}
	if n.Status != "" && !statuses[n.Status] {
		return fmt.Errorf("invalid status %q", n.Status)
	}
	if _, err := time.Parse(time.RFC3339Nano, n.Updated); err != nil {
		return fmt.Errorf("invalid updated timestamp: %w", err)
	}
	return nil
}
func Encode(n Note) ([]byte, error) {
	if err := validate(n); err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return nil, err
	}
	return []byte("---\n" + string(b) + "---\n" + n.Body), nil
}
func Decode(b []byte) (Note, error) {
	var n Note
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(b, []byte("---\n")) {
		return n, errors.New("missing YAML frontmatter")
	}
	end := bytes.Index(b[4:], []byte("\n---\n"))
	if end < 0 {
		return n, errors.New("unterminated YAML frontmatter")
	}
	if err := yaml.Unmarshal(b[4:4+end], &n); err != nil {
		return n, err
	}
	n.Body = string(b[4+end+5:])
	return n, validate(n)
}
func (s *Store) New(n Note) (string, error) {
	data, err := Encode(n)
	if err != nil {
		return "", err
	}
	folder := itemFolder
	if n.Kind == "project" {
		folder = "Projects"
	}
	if n.Kind == "run" {
		folder = runFolder
	}
	if n.Kind == "event" {
		folder = eventFolder
	}
	rel := filepath.Join("Thread", folder, n.ID+".md")
	p, err := s.path(rel)
	if err != nil {
		return "", err
	}
	if err = s.create(p, data); err != nil {
		return "", err
	}
	return p, nil
}

// Publish an already flushed file using an exclusive hard link. A duplicate ID
// never replaces an existing note, and readers never see a half-written note.
func (s *Store) create(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".thread-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Link(f.Name(), path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func readFileBefore(ctx context.Context, path string) ([]byte, error) {
	reader := readVaultFile
	result := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := reader(path)
		result <- struct {
			data []byte
			err  error
		}{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case got := <-result:
		return got.data, got.err
	}
}

func (s *Store) notesIn(dirs []string, allowDataless bool) ([]Note, VaultReadWarning, error) {
	var notes []Note
	var warning VaultReadWarning
	seen := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), vaultReadTimeout)
	defer cancel()
	for _, dir := range dirs {
		p, err := s.path(filepath.Join("Thread", dir))
		if err != nil {
			return nil, warning, err
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if os.IsNotExist(err) && path == p {
				return nil
			}
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in Thread records: %s", path)
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if datalessVaultFile(info) {
				warning.Skipped++
				if len(warning.Paths) < 3 {
					rel, relErr := filepath.Rel(s.Root, path)
					if relErr == nil {
						warning.Paths = append(warning.Paths, filepath.ToSlash(rel))
					}
				}
				if allowDataless {
					return nil
				}
				return warning
			}
			b, err := readFileBefore(ctx, path)
			if err != nil {
				return &VaultReadError{Path: path, Err: err}
			}
			n, err := Decode(b)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if seen[n.ID] {
				return fmt.Errorf("duplicate note id %s; resolve the sync conflict before updating", n.ID)
			}
			seen[n.ID] = true
			n.Path = path
			notes = append(notes, n)
			return nil
		})
		if err != nil {
			return nil, warning, err
		}
	}
	sort.Slice(notes, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, notes[i].Updated)
		b, _ := time.Parse(time.RFC3339Nano, notes[j].Updated)
		return a.After(b)
	})
	return notes, warning, nil
}

// Notes returns a complete view or an explicit error. It never silently hides
// records that are present in the vault but unavailable locally.
func (s *Store) Notes() ([]Note, error) {
	notes, _, err := s.notesIn([]string{"Projects", itemFolder, "Items", runFolder, "Runs", eventFolder}, false)
	return notes, err
}

// AvailableNotes is for human-facing, read-only views that can safely present
// partial results as long as they also present the warning.
func (s *Store) AvailableNotes() ([]Note, VaultReadWarning, error) {
	return s.notesIn([]string{"Projects", itemFolder, "Items", runFolder, "Runs", eventFolder}, true)
}

func (s *Store) projectNotes() ([]Note, error) {
	notes, _, err := s.notesIn([]string{"Projects"}, false)
	return notes, err
}
func (s *Store) Find(id string) (Note, error) {
	ns, err := s.Notes()
	if err != nil {
		return Note{}, err
	}
	for _, n := range ns {
		if n.ID == id {
			return n, nil
		}
	}
	return Note{}, fmt.Errorf("record %q not found", id)
}
func (s *Store) Link(n Note) string {
	rel, _ := filepath.Rel(s.Root, n.Path)
	return "[[" + strings.TrimSuffix(filepath.ToSlash(rel), ".md") + "|" + strings.ReplaceAll(strings.ReplaceAll(n.Title, "|", "-"), "]", "") + "]]"
}

// Display aliases may change without changing the linked note's identity.
func SameLink(a, b string) bool {
	target := func(x string) string {
		x = strings.TrimSuffix(strings.TrimPrefix(x, "[["), "]]")
		x, _, _ = strings.Cut(x, "|")
		return strings.TrimSuffix(x, ".md")
	}
	return target(a) == target(b)
}
func (s *Store) Project(id string) (Note, error) {
	notes, err := s.projectNotes()
	if err != nil {
		return Note{}, err
	}
	for _, n := range notes {
		if n.ID == id {
			if n.Kind != "project" {
				return n, errors.New("record is not a project")
			}
			return n, nil
		}
	}
	return Note{}, fmt.Errorf("record %q not found", id)
}
func (s *Store) Update(id string, change func(*Note) error) error {
	n, err := s.Find(id)
	if err != nil {
		return err
	}
	if n.Kind == "event" || n.Kind == "checkpoint" {
		return errors.New("events and checkpoints are immutable; create a linked capture instead")
	}
	lock := n.Path + ".thread-lock"
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("note is locked; inspect %s if a previous writer crashed: %w", lock, err)
	}
	f.Close()
	defer os.Remove(lock)
	original, err := os.ReadFile(n.Path)
	if err != nil {
		return err
	}
	current, err := Decode(original)
	if err != nil {
		return err
	}
	if err = change(&current); err != nil {
		return err
	}
	current.Updated = Now()
	data, err := Encode(current)
	if err != nil {
		return err
	}
	history, err := s.path(filepath.Join("Thread", ".history", n.ID, ID()+".md"))
	if err != nil {
		return err
	}
	if err = s.create(history, original); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(n.Path), ".thread-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	ce := temp.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	latest, err := os.ReadFile(n.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(original, latest) {
		return errors.New("note changed externally during update; original preserved in Thread/.history; retry after sync")
	}
	if err = os.Rename(temp.Name(), n.Path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(n.Path))
}
