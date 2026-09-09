package thread

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RecoveryManifest struct {
	Version  int               `json:"version"`
	Created  string            `json:"created"`
	Git      GitState          `json:"git"`
	Files    map[string]string `json:"sha256"`
	Excluded []string          `json:"excluded"`
}

// Snapshot preserves working file contents, not Git history or unsaved buffers.
// It never creates commits, stashes, or modifies the source checkout.
func Snapshot(repo, output string) (string, error) {
	state, err := Inspect(repo)
	if err != nil {
		return "", err
	}
	list, err := git(state.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	out, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(state.Root, out)
	if err != nil {
		return "", err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("snapshot output must be outside the source repository")
	}
	if err = os.MkdirAll(filepath.Dir(out), 0700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(out), ".thread-snapshot-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	z := zip.NewWriter(temp)
	manifest := RecoveryManifest{Version: 1, Created: Now(), Git: state, Files: map[string]string{}, Excluded: []string{"Git history/index, ignored files, unsaved editor buffers, symlinks, submodule contents"}}
	var total int64
	for _, name := range strings.Split(list, "\x00") {
		if name == "" {
			continue
		}
		if _, ok := manifest.Files[name]; ok {
			continue
		}
		if filepath.IsAbs(name) || filepath.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
			return "", errors.New("unsafe Git file path")
		}
		path := filepath.Join(state.Root, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			manifest.Excluded = append(manifest.Excluded, name)
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		if resolved != path {
			return "", fmt.Errorf("file path traverses a symlink: %s", name)
		}
		total += info.Size()
		if total > 128*1024*1024 {
			return "", errors.New("snapshot exceeds 128 MiB; narrow repository scope")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		header := &zip.FileHeader{Name: "files/" + filepath.ToSlash(name), Method: zip.Deflate}
		header.SetMode(info.Mode())
		w, err := z.CreateHeader(header)
		if err != nil {
			return "", err
		}
		if _, err = w.Write(data); err != nil {
			return "", err
		}
		manifest.Files[name] = Hash(data)
	}
	after, err := Inspect(state.Root)
	if err != nil {
		return "", err
	}
	again, err := git(state.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	if list != again || state.Head != after.Head || state.Changes != after.Changes {
		return "", errors.New("repository changed during snapshot; retry when writers are idle")
	}
	for name, hash := range manifest.Files {
		data, err := os.ReadFile(filepath.Join(state.Root, name))
		if err != nil || Hash(data) != hash {
			return "", errors.New("working file changed during snapshot; retry when writers are idle")
		}
	}
	w, err := z.Create("manifest.json")
	if err != nil {
		return "", err
	}
	if err = json.NewEncoder(w).Encode(manifest); err != nil {
		return "", err
	}
	if err = z.Close(); err != nil {
		return "", err
	}
	if err = temp.Sync(); err != nil {
		return "", err
	}
	if err = temp.Close(); err != nil {
		return "", err
	}
	if _, err = VerifySnapshot(temp.Name()); err != nil {
		return "", err
	}
	if err = os.Link(temp.Name(), out); err != nil {
		return "", err
	}
	return out, syncDir(filepath.Dir(out))
}
func VerifySnapshot(path string) (RecoveryManifest, error) {
	var manifest RecoveryManifest
	r, err := zip.OpenReader(path)
	if err != nil {
		return manifest, err
	}
	defer r.Close()
	files := map[string]*zip.File{}
	for _, f := range r.File {
		if _, ok := files[f.Name]; ok {
			return manifest, errors.New("duplicate archive entry")
		}
		files[f.Name] = f
	}
	m, ok := files["manifest.json"]
	if !ok {
		return manifest, errors.New("missing recovery manifest")
	}
	rd, err := m.Open()
	if err != nil {
		return manifest, err
	}
	err = json.NewDecoder(io.LimitReader(rd, 4*1024*1024)).Decode(&manifest)
	rd.Close()
	if err != nil {
		return manifest, err
	}
	if manifest.Version != 1 || manifest.Files == nil {
		return manifest, errors.New("invalid recovery manifest")
	}
	if len(files) != len(manifest.Files)+1 {
		return manifest, errors.New("unexpected archive entries")
	}
	var total uint64
	for name, hash := range manifest.Files {
		if filepath.IsAbs(name) || filepath.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
			return manifest, errors.New("unsafe archive path")
		}
		f, ok := files["files/"+name]
		if !ok {
			return manifest, fmt.Errorf("missing archived file: %s", name)
		}
		total += f.UncompressedSize64
		if total > 128*1024*1024 {
			return manifest, errors.New("archive exceeds verification limit")
		}
		rd, err := f.Open()
		if err != nil {
			return manifest, err
		}
		data, err := io.ReadAll(io.LimitReader(rd, 128*1024*1024+1))
		rd.Close()
		if err != nil {
			return manifest, err
		}
		if Hash(data) != hash {
			return manifest, fmt.Errorf("checksum mismatch: %s", name)
		}
	}
	return manifest, nil
}
