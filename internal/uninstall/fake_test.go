package uninstall_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/uninstall"
)

type node struct {
	name  string
	dir   bool
	link  bool
	size  int64
	mtime time.Time
}

// fakeSys is an in-memory PC: a tree of folders, files and links, a list of installed programs and a
// Recycle Bin that records what it was asked to move. Nothing touches the real disk.
type fakeSys struct {
	mu        sync.Mutex
	nodes     map[string]*node // lower-cased path -> node
	installed []uninstall.RegApp
	stuck     map[string]bool // paths the Recycle Bin cannot take (in use)
	recycled  []string        // every path the Recycle Bin was asked to move, in order
	removed   []string        // what it really moved
	measured  []string
}

var epoch = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

func newFake() *fakeSys { return &fakeSys{nodes: map[string]*node{}, stuck: map[string]bool{}} }

func key(p string) string { return strings.ToLower(filepath.Clean(p)) }

func (f *fakeSys) add(p string, n *node) {
	n.name = filepath.Base(p)
	if n.mtime.IsZero() {
		n.mtime = epoch
	}
	f.nodes[key(p)] = n
	// Every folder above exists too.
	for d := filepath.Dir(p); filepath.Dir(d) != d; d = filepath.Dir(d) {
		if _, ok := f.nodes[key(d)]; !ok {
			f.nodes[key(d)] = &node{name: filepath.Base(d), dir: true, mtime: epoch}
		}
	}
}

func (f *fakeSys) dir(p string)                  { f.add(p, &node{dir: true}) }
func (f *fakeSys) file(p string, size int64)     { f.add(p, &node{size: size}) }
func (f *fakeSys) link(p string, dir bool)       { f.add(p, &node{link: true, dir: dir}) }
func (f *fakeSys) has(p string) bool             { _, ok := f.nodes[key(p)]; return ok }
func (f *fakeSys) touched(p string, t time.Time) { f.nodes[key(p)].mtime = t }

func (f *fakeSys) ReadDir(p string) ([]uninstall.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.nodes[key(p)]; !ok || !n.dir || n.link {
		return nil, fs.ErrNotExist
	}
	var out []uninstall.Entry
	for k, n := range f.nodes {
		if filepath.Dir(k) == key(p) && k != key(p) {
			out = append(out, uninstall.Entry{Name: n.name, Dir: n.dir, Link: n.link, Size: n.size, ModTime: n.mtime})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeSys) Lstat(p string) (uninstall.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[key(p)]
	if !ok {
		return uninstall.Entry{}, fs.ErrNotExist
	}
	return uninstall.Entry{Name: n.name, Dir: n.dir, Link: n.link, Size: n.size, ModTime: n.mtime}, nil
}

func (f *fakeSys) Measure(_ context.Context, dir string) (int64, int, time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.measured = append(f.measured, dir)
	var bytes int64
	var files int
	var newest time.Time
	for k, n := range f.nodes {
		if strings.HasPrefix(k, key(dir)+string(filepath.Separator)) && !n.dir && !n.link {
			bytes += n.size
			files++
			if n.mtime.After(newest) {
				newest = n.mtime
			}
		}
	}
	return bytes, files, newest
}

func (f *fakeSys) recycle(paths []string, folders bool) (int, int64, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var moved int
	var freed int64
	var failed []string
	for _, p := range paths {
		f.recycled = append(f.recycled, p)
		n, ok := f.nodes[key(p)]
		if !ok || n.link || n.dir != folders || f.stuck[key(p)] {
			failed = append(failed, p)
			continue
		}
		for k, c := range f.nodes {
			if k == key(p) || strings.HasPrefix(k, key(p)+string(filepath.Separator)) {
				if !c.dir {
					freed += c.size
				}
				delete(f.nodes, k)
			}
		}
		moved++
		f.removed = append(f.removed, p)
	}
	return moved, freed, failed
}

func (f *fakeSys) RecycleFolders(_ context.Context, p []string) (int, int64, []string) {
	return f.recycle(p, true)
}
func (f *fakeSys) RecycleFiles(_ context.Context, p []string) (int, int64, []string) {
	return f.recycle(p, false)
}
func (f *fakeSys) Installed() []uninstall.RegApp { return f.installed }

// pc is a Windows-shaped machine laid out under a temporary folder's path (never created on disk).
type pc struct {
	*fakeSys
	env  uninstall.Env
	root string
}

func newPC(t *testing.T) *pc {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	env := uninstall.Env{
		Home: home, AppData: filepath.Join(home, "AppData", "Roaming"), LocalAppData: filepath.Join(home, "AppData", "Local"),
		ProgramData: filepath.Join(root, "ProgramData"), ProgramFiles: filepath.Join(root, "Program Files"),
		ProgramFilesX86: filepath.Join(root, "Program Files (x86)"), WinDir: filepath.Join(root, "Windows"),
		SystemDrive: root, Public: filepath.Join(root, "Users", "Public"), OneDrive: filepath.Join(home, "OneDrive"),
	}
	f := newFake()
	for _, d := range []string{env.AppData, env.LocalAppData, filepath.Join(env.LocalAppData, "Programs"), env.ProgramData, env.ProgramFiles,
		env.ProgramFilesX86, env.WinDir, filepath.Join(home, "Desktop"), filepath.Join(home, "Documents"), filepath.Join(home, "Downloads"),
		filepath.Join(env.Public, "Desktop"), filepath.Join(env.OneDrive, "Desktop"),
		filepath.Join(env.AppData, "Microsoft", "Windows", "Start Menu", "Programs"),
		filepath.Join(env.ProgramData, "Microsoft", "Windows", "Start Menu", "Programs")} {
		f.dir(d)
	}
	return &pc{fakeSys: f, env: env, root: root}
}

func (p *pc) finder() uninstall.Finder { return uninstall.Finder{Sys: p.fakeSys, Env: p.env} }

func (p *pc) path(parts ...string) string { return filepath.Join(parts...) }

func (p *pc) startMenu() string {
	return filepath.Join(p.env.AppData, "Microsoft", "Windows", "Start Menu", "Programs")
}
func (p *pc) startMenuAll() string {
	return filepath.Join(p.env.ProgramData, "Microsoft", "Windows", "Start Menu", "Programs")
}
func (p *pc) desktop() string { return filepath.Join(p.env.Home, "Desktop") }
