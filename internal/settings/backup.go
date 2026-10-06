package settings

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// FormatVersion is bumped when the archive layout changes.
	FormatVersion = 1
	maxFile       = 8 << 20
	maxTotal      = 64 << 20
	manifestName  = "manifest.json"
	extensionsRel = "extensions.txt"
)

// Roots maps a root name to its folder on this PC.
type Roots map[string]string

// DefaultRoots resolves the real folders of the current user.
func DefaultRoots() Roots {
	home, _ := os.UserHomeDir()
	docs := filepath.Join(home, "Documents")
	// OneDrive can redirect Documents; use it when that is where the files are.
	if od := os.Getenv("OneDrive"); od != "" {
		if _, err := os.Stat(filepath.Join(od, "Documents")); err == nil {
			if _, err := os.Stat(docs); err != nil {
				docs = filepath.Join(od, "Documents")
			}
		}
	}
	return Roots{
		RootAppData:      os.Getenv("APPDATA"),
		RootLocalAppData: os.Getenv("LOCALAPPDATA"),
		RootHome:         home,
		RootDocuments:    docs,
	}
}

// Runner runs an editor's CLI. It exists so tests do not need an editor.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

// Found is a set that has something to back up on this PC.
type Found struct {
	ID    string `json:"id"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// file is one concrete file of a set.
type file struct {
	root, rel, abs string
	size           int64
}

func (r Roots) files(s Set) []file {
	var out []file
	for _, p := range s.Paths {
		base := r[p.Root]
		if base == "" {
			continue
		}
		abs := filepath.Join(base, filepath.FromSlash(p.Rel))
		if !p.Dir {
			if fi, err := os.Lstat(abs); err == nil && fi.Mode().IsRegular() && fi.Size() <= maxFile {
				out = append(out, file{p.Root, p.Rel, abs, fi.Size()})
			}
			continue
		}
		_ = filepath.WalkDir(abs, func(fp string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return nil // symlinks and special files are never followed
			}
			fi, err := d.Info()
			if err != nil || fi.Size() > maxFile {
				return nil
			}
			rel, err := filepath.Rel(abs, fp)
			if err != nil {
				return nil
			}
			out = append(out, file{p.Root, path.Join(p.Rel, filepath.ToSlash(rel)), fp, fi.Size()})
			return nil
		})
	}
	return out
}

// Detect lists the sets that have files on this PC.
func Detect(r Roots, run Runner) []Found {
	var out []Found
	for _, s := range Sets {
		fl := r.files(s)
		f := Found{ID: s.ID, Files: len(fl)}
		for _, x := range fl {
			f.Bytes += x.size
		}
		if s.Extensions != nil && len(listExtensions(s, run)) > 0 {
			f.Files++
		}
		if f.Files > 0 {
			out = append(out, f)
		}
	}
	return out
}

func listExtensions(s Set, run Runner) []string {
	if s.Extensions == nil || run == nil {
		return nil
	}
	b, err := run.Run(s.Extensions.Exe, s.Extensions.List...)
	if err != nil {
		return nil
	}
	var ids []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); extensionID.MatchString(l) {
			ids = append(ids, l)
		}
	}
	sort.Strings(ids)
	return ids
}

// extensionID is publisher.name; anything else is never passed to a command.
var extensionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*\.[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Manifest describes an archive.
type Manifest struct {
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Sets      []Found   `json:"sets"`
}

// Backup writes a zip with the chosen sets.
func Backup(w io.Writer, ids []string, r Roots, run Runner) (*Manifest, error) {
	zw := zip.NewWriter(w)
	m := &Manifest{Format: FormatVersion, CreatedAt: time.Now().UTC()}
	var total int64
	for _, id := range ids {
		s := find(id)
		if s == nil {
			return nil, fmt.Errorf("unknown settings set %q", id)
		}
		f := Found{ID: id}
		for _, x := range r.files(*s) {
			if total += x.size; total > maxTotal {
				return nil, errors.New("the settings are too large to back up")
			}
			b, err := os.ReadFile(x.abs)
			if err != nil {
				continue
			}
			if err := put(zw, path.Join(id, x.root, x.rel), b); err != nil {
				return nil, err
			}
			f.Files++
			f.Bytes += x.size
		}
		if exts := listExtensions(*s, run); len(exts) > 0 {
			if err := put(zw, path.Join(id, extensionsRel), []byte(strings.Join(exts, "\n")+"\n")); err != nil {
				return nil, err
			}
			f.Files++
		}
		if f.Files > 0 {
			m.Sets = append(m.Sets, f)
		}
	}
	if len(m.Sets) == 0 {
		return nil, errors.New("nothing to back up")
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	if err := put(zw, manifestName, mb); err != nil {
		return nil, err
	}
	return m, zw.Close()
}

func put(zw *zip.Writer, name string, b []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ReadManifest opens an archive and returns what it says it holds. It trusts
// nothing in it: Restore re-validates every entry.
func ReadManifest(ra io.ReaderAt, size int64) (*Manifest, error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, errors.New("not a WinForge settings backup")
	}
	for _, f := range zr.File {
		if f.Name != manifestName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			break
		}
		var m Manifest
		err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&m)
		rc.Close()
		if err != nil || m.Format < 1 {
			break
		}
		if m.Format > FormatVersion {
			return nil, fmt.Errorf("this backup is from a newer WinForge (format %d)", m.Format)
		}
		known := []Found{}
		for _, s := range m.Sets {
			if find(s.ID) != nil {
				known = append(known, s)
			}
		}
		m.Sets = known
		return &m, nil
	}
	return nil, errors.New("not a WinForge settings backup")
}

// Result says what a restore did.
type Result struct {
	Restored   int      `json:"restored"`
	Unchanged  int      `json:"unchanged"`
	BackedUp   int      `json:"backedUp"`
	Extensions int      `json:"extensions"`
	Skipped    []string `json:"skipped"`
}

// BackupSuffix is added to a file that a restore replaced.
const BackupSuffix = ".winforge-bak"

// Restore writes the chosen sets from the archive back to this PC. Every entry
// must name a file that the set list allows, so a crafted archive cannot write
// anywhere else; a file that differs is kept next to the new one with
// BackupSuffix.
func Restore(ra io.ReaderAt, size int64, ids []string, r Roots, run Runner) (*Result, error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, errors.New("not a WinForge settings backup")
	}
	want := map[string]bool{}
	for _, id := range ids {
		if find(id) == nil {
			return nil, fmt.Errorf("unknown settings set %q", id)
		}
		want[id] = true
	}
	res := &Result{Skipped: []string{}}
	var total int64
	for _, f := range zr.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) != 2 || !want[parts[0]] {
			continue
		}
		s := find(parts[0])
		if parts[1] == extensionsRel {
			res.Extensions += installExtensions(f, *s, run)
			continue
		}
		root, rel, ok := splitRootRel(parts[1])
		target, allowed := resolve(*s, r, root, rel)
		if !ok || !allowed || f.UncompressedSize64 > maxFile {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxFile+1))
		rc.Close()
		if err != nil || len(b) > maxFile {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		if total += int64(len(b)); total > maxTotal {
			return res, errors.New("the backup is too large")
		}
		old, err := os.ReadFile(target)
		switch {
		case err == nil && bytes.Equal(old, b):
			res.Unchanged++
			continue
		case err == nil:
			if os.WriteFile(target+BackupSuffix, old, 0o644) == nil {
				res.BackedUp++
			}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		res.Restored++
	}
	return res, nil
}

func splitRootRel(s string) (root, rel string, ok bool) {
	i := strings.Index(s, "/")
	if i <= 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// resolve maps an archive entry to a real path, only when the set lists it.
func resolve(s Set, r Roots, root, rel string) (string, bool) {
	if rel == "" || strings.Contains(rel, "\\") || strings.Contains(rel, ":") || strings.HasPrefix(rel, "/") ||
		path.Clean(rel) != rel || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", false
	}
	base := r[root]
	if base == "" {
		return "", false
	}
	for _, p := range s.Paths {
		if p.Root != root {
			continue
		}
		if (!p.Dir && rel == p.Rel) || (p.Dir && strings.HasPrefix(rel, p.Rel+"/")) {
			return filepath.Join(base, filepath.FromSlash(rel)), true
		}
	}
	return "", false
}

func installExtensions(f *zip.File, s Set, run Runner) int {
	if s.Extensions == nil || run == nil {
		return 0
	}
	rc, err := f.Open()
	if err != nil {
		return 0
	}
	defer rc.Close()
	b, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if !extensionID.MatchString(l) {
			continue
		}
		args := append(append([]string{}, s.Extensions.Install...), l, "--force")
		if _, err := run.Run(s.Extensions.Exe, args...); err == nil {
			n++
		}
	}
	return n
}
