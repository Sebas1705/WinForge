package offlinekit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// MemFS is an in-memory disk for tests (here and in the app's binding tests). Paths are compared
// case-insensitively, like Windows, and a path can be marked as a link.
type MemFS struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
	links map[string]bool   // paths that are links (to a file or a folder)
	orig  map[string]string // lower-cased path -> the path as written, so names keep their case
	tmpN  int
	// Capacity is the size of the drive in bytes; 0 means unlimited. Free space is Capacity minus what the files take.
	Capacity int64
	// FailOpen makes Open fail for the paths (lower-cased) it holds, as a removed stick would.
	FailOpen map[string]error
}

// NewMemFS makes an empty disk.
func NewMemFS() *MemFS {
	return &MemFS{files: map[string][]byte{}, dirs: map[string]bool{}, links: map[string]bool{}, orig: map[string]string{}, FailOpen: map[string]error{}}
}

// name is the last element of a path as it was written, not as it is compared.
func (m *MemFS) name(k string) string {
	if o, ok := m.orig[k]; ok {
		return filepath.Base(o)
	}
	return filepath.Base(k)
}

func key(p string) string { return strings.ToLower(filepath.Clean(p)) }

func (m *MemFS) mkdirs(p string) {
	for p = filepath.Clean(p); ; p = filepath.Dir(p) {
		m.dirs[key(p)] = true
		m.orig[key(p)] = p
		if filepath.Dir(p) == p {
			return
		}
	}
}

// AddFile writes a file, making its folders.
func (m *MemFS) AddFile(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mkdirs(filepath.Dir(path))
	m.files[key(path)] = append([]byte(nil), data...)
	m.orig[key(path)] = path
}

// AddDir makes a folder.
func (m *MemFS) AddDir(path string) { m.mu.Lock(); defer m.mu.Unlock(); m.mkdirs(path) }

// AddLink makes a link: a file-like link, or a folder-like one when dir is true.
func (m *MemFS) AddLink(path string, dir bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mkdirs(filepath.Dir(path))
	m.links[key(path)] = true
	m.orig[key(path)] = path
	if !dir {
		m.files[key(path)] = []byte("link")
	} else {
		m.dirs[key(path)] = true
	}
}

// Read returns a file's bytes.
func (m *MemFS) Read(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[key(path)]
	return append([]byte(nil), b...), ok
}

// Exists reports whether a file or folder is there.
func (m *MemFS) Exists(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, f := m.files[key(path)]
	return f || m.dirs[key(path)]
}

// Names lists the names directly inside a folder, sorted.
func (m *MemFS) Names(dir string) []string {
	ents, _ := m.ReadDir(dir)
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name)
	}
	sort.Strings(out)
	return out
}

func (m *MemFS) used() int64 {
	var n int64
	for _, b := range m.files {
		n += int64(len(b))
	}
	return n
}

func (m *MemFS) Lstat(path string) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(path)
	if b, ok := m.files[k]; ok {
		return Info{Name: m.name(k), Link: m.links[k], Size: int64(len(b))}, nil
	}
	if m.dirs[k] {
		link := m.links[k]
		return Info{Name: m.name(k), Dir: !link, Link: link}, nil
	}
	return Info{}, os.ErrNotExist
}

func (m *MemFS) ReadDir(dir string) ([]Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := key(dir)
	if !m.dirs[d] {
		return nil, os.ErrNotExist
	}
	seen := map[string]Info{}
	add := func(full string, isDir bool, size int64) {
		if filepath.Dir(full) != d || full == d {
			return
		}
		link := m.links[full]
		seen[full] = Info{Name: m.name(full), Dir: isDir && !link, Link: link, Size: size}
	}
	for p, b := range m.files {
		add(p, false, int64(len(b)))
	}
	for p := range m.dirs {
		if _, isFile := m.files[p]; !isFile {
			add(p, true, 0)
		}
	}
	out := make([]Info, 0, len(seen))
	for _, i := range seen {
		out = append(out, i)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *MemFS) Open(path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.FailOpen[key(path)]; ok {
		return nil, err
	}
	b, ok := m.files[key(path)]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

type memWriter struct {
	m    *MemFS
	path string
	buf  bytes.Buffer
}

func (w *memWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *memWriter) Close() error {
	w.m.mu.Lock()
	defer w.m.mu.Unlock()
	if w.m.Capacity > 0 && w.m.used()+int64(w.buf.Len()) > w.m.Capacity {
		return errors.New("no space left on device")
	}
	w.m.files[key(w.path)] = w.buf.Bytes()
	w.m.orig[key(w.path)] = w.path
	return nil
}

func (m *MemFS) Create(path string) (io.WriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[key(path)]; ok {
		return nil, os.ErrExist
	}
	if !m.dirs[key(filepath.Dir(path))] {
		return nil, os.ErrNotExist
	}
	return &memWriter{m: m, path: path}, nil
}

func (m *MemFS) MkdirAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mkdirs(path)
	return nil
}

func (m *MemFS) RemoveAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(path)
	prefix := k + string(filepath.Separator)
	for p := range m.files {
		if p == k || strings.HasPrefix(p, prefix) {
			delete(m.files, p)
			delete(m.links, p)
		}
	}
	for p := range m.dirs {
		if p == k || strings.HasPrefix(p, prefix) {
			delete(m.dirs, p)
			delete(m.links, p)
		}
	}
	return nil
}

func (m *MemFS) Rename(oldPath, newPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[key(oldPath)]
	if !ok {
		return os.ErrNotExist
	}
	delete(m.files, key(oldPath))
	m.files[key(newPath)] = b
	m.orig[key(newPath)] = newPath
	return nil
}

func (m *MemFS) MkdirTemp(pattern string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tmpN++
	dir := filepath.Join(os.TempDir(), strings.ReplaceAll(pattern, "*", fmt.Sprint(m.tmpN)))
	m.mkdirs(dir)
	return dir, nil
}

func (m *MemFS) FreeBytes(string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Capacity == 0 {
		return 1 << 50, nil
	}
	return m.Capacity - m.used(), nil
}

// FakePackage is what a FakeDownloader writes for one winget id.
type FakePackage struct {
	Files map[string][]byte
	// Code is the exit code to report; Err is returned instead of running.
	Code int
	Err  error
}

// FakeDownloader writes canned packages into a MemFS instead of downloading anything.
type FakeDownloader struct {
	FS       *MemFS
	Packages map[string]FakePackage
	// OnCall runs when a download starts, with its position (0-based); tests use it to cancel mid-way.
	OnCall func(n int, id string)
	Calls  []string
}

// Download implements Downloader.
func (d *FakeDownloader) Download(ctx context.Context, id, dir string, onLine func(string)) (int, error) {
	d.Calls = append(d.Calls, id)
	if d.OnCall != nil {
		d.OnCall(len(d.Calls)-1, id)
	}
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	p, ok := d.Packages[id]
	if !ok {
		return 0x8A150014, nil
	}
	if p.Err != nil {
		return -1, p.Err
	}
	onLine("50 %")
	if p.Code != 0 {
		return p.Code, nil
	}
	for name, data := range p.Files {
		d.FS.AddFile(filepath.Join(dir, name), data)
	}
	return 0, nil
}
