package uninstall

import (
	"context"
	"io/fs"
	"os"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// osSystem reads the real disk and sends to the Recycle Bin through storage's recycler, which refuses
// links and counts a path as moved only when it is really gone. Only the registry part is Windows-specific
// (see registry_windows.go), so the rest is testable anywhere with a temporary folder.
type osSystem struct {
	run       storage.Runner
	installed func() []RegApp
}

// NewSystem is the real machine. run executes the Recycle Bin command; nil moves nothing.
func NewSystem(run storage.Runner) System { return osSystem{run, registryApps} }

// NewSystemWithPrograms is the real disk with a given list of installed programs instead of this PC's
// registry, so tests do not depend on what happens to be installed.
func NewSystemWithPrograms(run storage.Runner, installed func() []RegApp) System {
	return osSystem{run, installed}
}

func isLink(m fs.FileMode) bool {
	return m&(os.ModeSymlink|os.ModeIrregular|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) != 0
}

func (osSystem) ReadDir(path string) ([]Entry, error) {
	ds, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(ds))
	for _, d := range ds {
		e := Entry{Name: d.Name(), Dir: d.IsDir(), Link: isLink(d.Type())}
		if info, err := d.Info(); err == nil {
			e.Size, e.ModTime = info.Size(), info.ModTime()
		}
		out = append(out, e)
	}
	return out, nil
}

func (osSystem) Lstat(path string) (Entry, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Name: st.Name(), Dir: st.IsDir(), Link: isLink(st.Mode()), Size: st.Size(), ModTime: st.ModTime()}, nil
}

// maxMeasured bounds how many files one folder's size will walk, so a huge folder cannot stall the page.
const maxMeasured = 500000

func (osSystem) Measure(ctx context.Context, dir string) (bytes int64, files int, newest time.Time) {
	var walk func(d string)
	walk = func(d string) {
		if ctx.Err() != nil || files >= maxMeasured {
			return
		}
		ds, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range ds {
			if isLink(e.Type()) {
				continue
			}
			p := d + string(os.PathSeparator) + e.Name()
			if e.IsDir() {
				walk(p)
				continue
			}
			if info, err := e.Info(); err == nil {
				bytes += info.Size()
				files++
				if info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
		}
	}
	walk(dir)
	return bytes, files, newest
}

func (s osSystem) RecycleFolders(ctx context.Context, paths []string) (int, int64, []string) {
	return storage.RecycleFolders(ctx, s.run, paths)
}

func (s osSystem) RecycleFiles(ctx context.Context, paths []string) (int, int64, []string) {
	return storage.Recycle(ctx, s.run, paths)
}

func (s osSystem) Installed() []RegApp { return s.installed() }
