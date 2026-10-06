package settings

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	// FormatVersion is bumped when the archive layout changes. Version 1
	// archives (settings only, no checksums) still read.
	FormatVersion = 2
	maxFile       = 8 << 20
	maxSets       = 64 << 20
	// maxFolders bounds the folders a person adds themselves.
	maxFolders    = int64(2) << 30
	manifestName  = "manifest.json"
	profileName   = "profile.json"
	extensionsRel = "extensions.txt"
	foldersDir    = "folders"
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
		// A pattern ("JetBrains/*/options") expands to the folders that exist.
		matches, err := filepath.Glob(filepath.Join(base, filepath.FromSlash(p.Rel)))
		if err != nil {
			continue
		}
		for _, abs := range matches {
			rel, err := filepath.Rel(base, abs)
			if err != nil {
				continue
			}
			rel = filepath.ToSlash(rel)
			if !p.Dir {
				if fi, err := os.Lstat(abs); err == nil && fi.Mode().IsRegular() && fi.Size() <= maxFile {
					out = append(out, file{p.Root, rel, abs, fi.Size()})
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
				sub, err := filepath.Rel(base, fp)
				if err != nil {
					return nil
				}
				out = append(out, file{p.Root, filepath.ToSlash(sub), fp, fi.Size()})
				return nil
			})
		}
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

// FolderInfo is a folder the person added to a backup.
type FolderInfo struct {
	Name    string `json:"name"` // what it was called; restored under this name
	Dir     string `json:"dir"`  // its prefix inside the archive
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Skipped int    `json:"skipped"` // files that could not be read (in use, no access)
}

// Manifest describes an archive.
type Manifest struct {
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Sets      []Found   `json:"sets"`
	// Host is the PC the backup came from and App the WinForge that made it,
	// so a restore can say where its contents are from.
	Host string `json:"host,omitempty"`
	App  string `json:"app,omitempty"`
	// HasProfile is true when profile.json (the apps installed at the time) is inside.
	HasProfile bool         `json:"hasProfile,omitempty"`
	Apps       int          `json:"apps,omitempty"`
	Folders    []FolderInfo `json:"folders,omitempty"`
	// Sums holds the SHA-256 of every other entry, so damage or editing is noticed.
	Sums map[string]string `json:"sums,omitempty"`
}

// Options says what a backup holds.
type Options struct {
	IDs   []string // settings sets
	Roots Roots
	Run   Runner
	// Profile is the WinForge profile file of the installed apps, if wanted.
	Profile []byte
	Apps    int
	// Folders are directories the person chose. They are restored into a
	// folder the person chooses then, never to where they came from.
	Folders []string
	Host    string
	App     string
}

type writer struct {
	zw   *zip.Writer
	sums map[string]string
}

func (w *writer) put(name string, b []byte) error {
	h := sha256.Sum256(b)
	w.sums[name] = hex.EncodeToString(h[:])
	e, err := w.zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = e.Write(b)
	return err
}

func (w *writer) stream(name string, src io.Reader) (int64, error) {
	e, err := w.zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(e, h), src)
	if err != nil {
		return n, err
	}
	w.sums[name] = hex.EncodeToString(h.Sum(nil))
	return n, nil
}

// Backup writes a zip with the chosen settings sets only.
func Backup(w io.Writer, ids []string, r Roots, run Runner) (*Manifest, error) {
	return Create(w, Options{IDs: ids, Roots: r, Run: run})
}

// Create writes an archive. The manifest comes last, carrying the checksums.
func Create(out io.Writer, o Options) (*Manifest, error) {
	w := &writer{zw: zip.NewWriter(out), sums: map[string]string{}}
	m := &Manifest{Format: FormatVersion, CreatedAt: time.Now().UTC(), Host: o.Host, App: o.App}
	var total int64
	for _, id := range o.IDs {
		s := find(id)
		if s == nil {
			return nil, fmt.Errorf("unknown settings set %q", id)
		}
		f := Found{ID: id}
		for _, x := range o.Roots.files(*s) {
			if total += x.size; total > maxSets {
				return nil, errors.New("the settings are too large to back up")
			}
			b, err := os.ReadFile(x.abs)
			if err != nil {
				continue
			}
			if err := w.put(path.Join(id, x.root, x.rel), b); err != nil {
				return nil, err
			}
			f.Files++
			f.Bytes += x.size
		}
		if exts := listExtensions(*s, o.Run); len(exts) > 0 {
			if err := w.put(path.Join(id, extensionsRel), []byte(strings.Join(exts, "\n")+"\n")); err != nil {
				return nil, err
			}
			f.Files++
		}
		if f.Files > 0 {
			m.Sets = append(m.Sets, f)
		}
	}
	if len(o.Profile) > 0 {
		if err := w.put(profileName, o.Profile); err != nil {
			return nil, err
		}
		m.HasProfile, m.Apps = true, o.Apps
	}
	var bytesLeft = maxFolders
	for i, dir := range o.Folders {
		fi, err := addFolder(w, dir, fmt.Sprintf("%s/%d", foldersDir, i+1), &bytesLeft)
		if err != nil {
			return nil, err
		}
		m.Folders = append(m.Folders, fi)
	}
	if len(m.Sets) == 0 && !m.HasProfile && len(m.Folders) == 0 {
		return nil, errors.New("nothing to back up")
	}
	m.Sums = w.sums
	mb, _ := json.MarshalIndent(m, "", "  ")
	e, err := w.zw.CreateHeader(&zip.FileHeader{Name: manifestName, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return nil, err
	}
	if _, err := e.Write(mb); err != nil {
		return nil, err
	}
	return m, w.zw.Close()
}

func addFolder(w *writer, dir, prefix string, bytesLeft *int64) (FolderInfo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return FolderInfo{}, err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return FolderInfo{}, fmt.Errorf("%q is not a folder", dir)
	}
	fi := FolderInfo{Name: safeName(filepath.Base(abs)), Dir: prefix}
	err = filepath.WalkDir(abs, func(fp string, d fs.DirEntry, werr error) error {
		if werr != nil {
			fi.Skipped++
			return nil
		}
		if !d.Type().IsRegular() { // directories descend; links and devices are never followed
			return nil
		}
		rel, err := filepath.Rel(abs, fp)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			fi.Skipped++
			return nil
		}
		if *bytesLeft -= info.Size(); *bytesLeft < 0 {
			return errors.New("the folders are too large for one backup (2 GB limit)")
		}
		f, err := os.Open(fp)
		if err != nil {
			fi.Skipped++
			*bytesLeft += info.Size()
			return nil
		}
		defer f.Close()
		n, err := w.stream(path.Join(prefix, filepath.ToSlash(rel)), f)
		if err != nil {
			return err
		}
		fi.Files++
		fi.Bytes += n
		return nil
	})
	return fi, err
}

// safeName makes a folder's name usable as a single path segment.
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(s, " .")
	if s == "" {
		return "folder"
	}
	return s
}

func open(ra io.ReaderAt, size int64) (*zip.Reader, error) {
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, errors.New("not a WinForge backup")
	}
	return zr, nil
}

// ReadManifest opens an archive and returns what it says it holds. It trusts
// nothing in it: Restore re-validates every entry.
func ReadManifest(ra io.ReaderAt, size int64) (*Manifest, error) {
	zr, err := open(ra, size)
	if err != nil {
		return nil, err
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
		err = json.NewDecoder(io.LimitReader(rc, 8<<20)).Decode(&m)
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
	return nil, errors.New("not a WinForge backup")
}

// Integrity is the result of checking an archive against its checksums.
type Integrity struct {
	Files    int      `json:"files"`
	Checked  bool     `json:"checked"` // false for version-1 archives, which carry no checksums
	Damaged  []string `json:"damaged"`
	Missing  []string `json:"missing"`
	Unlisted []string `json:"unlisted"` // entries the manifest does not know: added after the backup was made
}

// OK is true when nothing is wrong.
func (i *Integrity) OK() bool { return len(i.Damaged)+len(i.Missing)+len(i.Unlisted) == 0 }

// Verify reads every entry and compares it with the manifest's checksums.
func Verify(ra io.ReaderAt, size int64) (*Integrity, error) {
	m, err := ReadManifest(ra, size)
	if err != nil {
		return nil, err
	}
	zr, _ := open(ra, size)
	res := &Integrity{Checked: len(m.Sums) > 0, Damaged: []string{}, Missing: []string{}, Unlisted: []string{}}
	seen := map[string]bool{}
	for _, f := range zr.File {
		if f.Name == manifestName {
			continue
		}
		res.Files++
		seen[f.Name] = true
		want, listed := m.Sums[f.Name]
		if !res.Checked {
			continue
		}
		if !listed {
			res.Unlisted = append(res.Unlisted, f.Name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			res.Damaged = append(res.Damaged, f.Name)
			continue
		}
		h := sha256.New()
		_, err = io.Copy(h, rc) // the zip reader also checks its own CRC at the end
		rc.Close()
		if err != nil || hex.EncodeToString(h.Sum(nil)) != want {
			res.Damaged = append(res.Damaged, f.Name)
		}
	}
	for name := range m.Sums {
		if !seen[name] {
			res.Missing = append(res.Missing, name)
		}
	}
	sort.Strings(res.Missing)
	return res, nil
}

// Profile returns profile.json from the archive, or nil when it has none.
func Profile(ra io.ReaderAt, size int64) ([]byte, error) {
	zr, err := open(ra, size)
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name == profileName {
			return readEntry(f, 4<<20)
		}
	}
	return nil, nil
}

func readEntry(f *zip.File, limit int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > limit {
		return nil, errors.New("entry too large")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("entry too large")
	}
	return b, nil
}

// Change is what restoring one file would do.
type Change struct {
	Set   string `json:"set"`
	File  string `json:"file"`  // root/relative path, for display
	State string `json:"state"` // new, changed, same
}

// Diff compares the archive with this PC without writing anything, so a
// restore can show what it would add and what it would replace.
func Diff(ra io.ReaderAt, size int64, ids []string, r Roots) ([]Change, error) {
	zr, err := open(ra, size)
	if err != nil {
		return nil, err
	}
	want := wanted(ids)
	var out []Change
	for _, f := range zr.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) != 2 || !want[parts[0]] || parts[1] == extensionsRel {
			continue
		}
		s := find(parts[0])
		root, rel, ok := splitRootRel(parts[1])
		target, allowed := resolve(*s, r, root, rel)
		if !ok || !allowed {
			continue
		}
		b, err := readEntry(f, maxFile)
		if err != nil {
			continue
		}
		state := "new"
		if old, err := os.ReadFile(target); err == nil {
			state = "changed"
			if bytes.Equal(old, b) {
				state = "same"
			}
		}
		out = append(out, Change{Set: parts[0], File: root + "/" + rel, State: state})
	}
	return out, nil
}

func wanted(ids []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range ids {
		if find(id) != nil {
			m[id] = true
		}
	}
	return m
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
// BackupSuffix. An entry whose checksum no longer matches is not written.
func Restore(ra io.ReaderAt, size int64, ids []string, r Roots, run Runner) (*Result, error) {
	zr, err := open(ra, size)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if find(id) == nil {
			return nil, fmt.Errorf("unknown settings set %q", id)
		}
	}
	want := wanted(ids)
	sums := manifestSums(ra, size)
	res := &Result{Skipped: []string{}}
	var total int64
	for _, f := range zr.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) != 2 || !want[parts[0]] {
			continue
		}
		s := find(parts[0])
		if parts[1] == extensionsRel {
			if ok := intact(f, sums); !ok {
				res.Skipped = append(res.Skipped, f.Name)
				continue
			}
			res.Extensions += installExtensions(f, *s, run)
			continue
		}
		root, rel, ok := splitRootRel(parts[1])
		target, allowed := resolve(*s, r, root, rel)
		if !ok || !allowed {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		b, err := readEntry(f, maxFile)
		if err != nil || !matchesSum(f.Name, b, sums) {
			res.Skipped = append(res.Skipped, f.Name)
			continue
		}
		if total += int64(len(b)); total > maxSets {
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

func manifestSums(ra io.ReaderAt, size int64) map[string]string {
	m, err := ReadManifest(ra, size)
	if err != nil {
		return nil
	}
	return m.Sums
}

func matchesSum(name string, b []byte, sums map[string]string) bool {
	if sums == nil {
		return true // version-1 archive: nothing to compare with
	}
	want, ok := sums[name]
	if !ok {
		return false
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) == want
}

func intact(f *zip.File, sums map[string]string) bool {
	b, err := readEntry(f, 1<<20)
	return err == nil && matchesSum(f.Name, b, sums)
}

func splitRootRel(s string) (root, rel string, ok bool) {
	i := strings.Index(s, "/")
	if i <= 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// cleanRel is true for a relative path that stays inside whatever it is joined to.
func cleanRel(rel string) bool {
	return rel != "" && !strings.Contains(rel, "\\") && !strings.Contains(rel, ":") && !strings.HasPrefix(rel, "/") &&
		path.Clean(rel) == rel && rel != ".." && !strings.HasPrefix(rel, "../")
}

// resolve maps an archive entry to a real path, only when the set lists it.
func resolve(s Set, r Roots, root, rel string) (string, bool) {
	if !cleanRel(rel) {
		return "", false
	}
	base := r[root]
	if base == "" {
		return "", false
	}
	for _, p := range s.Paths {
		if p.Root == root && p.matches(rel) {
			return filepath.Join(base, filepath.FromSlash(rel)), true
		}
	}
	return "", false
}

func installExtensions(f *zip.File, s Set, run Runner) int {
	if s.Extensions == nil || run == nil {
		return 0
	}
	b, err := readEntry(f, 1<<20)
	if err != nil {
		return 0
	}
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

// FolderResult says what restoring folders did.
type FolderResult struct {
	Written int      `json:"written"`
	Same    int      `json:"same"`
	Kept    int      `json:"kept"` // already there and different: left alone
	Skipped []string `json:"skipped"`
	Dest    string   `json:"dest"`
}

// RestoreFolders writes the chosen folders of the archive under dest, each in
// a folder of its own name. Nothing that already exists is overwritten.
func RestoreFolders(ra io.ReaderAt, size int64, dest string, names []string) (*FolderResult, error) {
	m, err := ReadManifest(ra, size)
	if err != nil {
		return nil, err
	}
	zr, _ := open(ra, size)
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		return nil, errors.New("choose a folder to restore into")
	}
	pick := map[string]bool{}
	for _, n := range names {
		pick[n] = true
	}
	res := &FolderResult{Skipped: []string{}, Dest: dest}
	for _, fo := range m.Folders {
		if !pick[fo.Name] {
			continue
		}
		// The prefix comes from the manifest, so it is checked like any entry.
		if !regexp.MustCompile(`^folders/\d{1,3}$`).MatchString(fo.Dir) {
			continue
		}
		base := filepath.Join(dest, safeName(fo.Name))
		for _, f := range zr.File {
			rel, ok := strings.CutPrefix(f.Name, fo.Dir+"/")
			if !ok {
				continue
			}
			if !cleanRel(rel) {
				res.Skipped = append(res.Skipped, f.Name)
				continue
			}
			target := filepath.Join(base, filepath.FromSlash(rel))
			if exists, same := sameFile(target, f); exists {
				if same {
					res.Same++
				} else {
					res.Kept++
				}
				continue
			}
			if err := writeEntry(f, target, m.Sums[f.Name]); err != nil {
				res.Skipped = append(res.Skipped, f.Name)
				continue
			}
			res.Written++
		}
	}
	return res, nil
}

// sameFile reports whether target exists and, if so, has the entry's content.
func sameFile(target string, f *zip.File) (exists, same bool) {
	st, err := os.Lstat(target)
	if err != nil {
		return false, false
	}
	if !st.Mode().IsRegular() || st.Size() != int64(f.UncompressedSize64) {
		return true, false
	}
	rc, err := f.Open()
	if err != nil {
		return true, false
	}
	defer rc.Close()
	a, err := os.Open(target)
	if err != nil {
		return true, false
	}
	defer a.Close()
	ha, hb := sha256.New(), sha256.New()
	if _, err := io.Copy(ha, a); err != nil {
		return true, false
	}
	if _, err := io.Copy(hb, rc); err != nil {
		return true, false
	}
	return true, bytes.Equal(ha.Sum(nil), hb.Sum(nil))
}

// writeEntry streams an entry to disk through a temporary name, so a failed or
// altered copy never leaves a half-written file under the real one.
func writeEntry(f *zip.File, target, wantSum string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := target + ".winforge-part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && wantSum != "" && hex.EncodeToString(h.Sum(nil)) != wantSum {
		err = errors.New("checksum mismatch")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
}
