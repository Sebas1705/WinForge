package storage

import (
	"container/heap"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BigFile is one of the largest files under a folder.
type BigFile struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	Modified  time.Time `json:"modified"`
}

// FolderSize is the total size of one folder.
type FolderSize struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Files     int    `json:"files"`
}

// skipDirs are folders nothing useful or removable is found in.
var skipDirs = map[string]bool{
	"$recycle.bin":              true,
	"system volume information": true,
}

func checkRoot(root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("choose a full folder path")
	}
	st, err := os.Stat(root)
	if err != nil {
		return errors.New("that folder cannot be read")
	}
	if !st.IsDir() {
		return errors.New("that is not a folder")
	}
	return nil
}

// walkAll visits regular files under root, skipping links and the folders in skip.
func walkAll(ctx context.Context, root string, skip func(name string) bool, visit func(path string, info fs.FileInfo)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if werr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !plain(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && skip != nil && skip(strings.ToLower(d.Name())) {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		visit(p, info)
		return nil
	})
}

type fileHeap []BigFile

func (h fileHeap) Len() int           { return len(h) }
func (h fileHeap) Less(i, j int) bool { return h[i].SizeBytes < h[j].SizeBytes }
func (h fileHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *fileHeap) Push(x any)        { *h = append(*h, x.(BigFile)) }
func (h *fileHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// BigFiles returns the largest files under root, biggest first. limit is
// clamped to 1..500. Only a bounded heap is kept, so a drive with millions of
// files costs no more memory than one with a hundred.
func BigFiles(ctx context.Context, root string, limit int, progress ProgressFunc) ([]BigFile, error) {
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	limit = max(1, min(limit, 500))
	th := newThrottle("big", progress)
	h := &fileHeap{}
	err := walkAll(ctx, root, func(n string) bool { return skipDirs[n] }, func(p string, info fs.FileInfo) {
		th.add(1, info.Size(), p)
		if info.Size() <= 0 {
			return
		}
		f := BigFile{Path: p, Name: info.Name(), SizeBytes: info.Size(), Modified: info.ModTime()}
		if h.Len() < limit {
			heap.Push(h, f)
		} else if f.SizeBytes > (*h)[0].SizeBytes {
			(*h)[0] = f
			heap.Fix(h, 0)
		}
	})
	th.done()
	out := make([]BigFile, len(*h))
	copy(out, *h)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SizeBytes != out[j].SizeBytes {
			return out[i].SizeBytes > out[j].SizeBytes
		}
		return out[i].Path < out[j].Path
	})
	return out, err
}

// FolderSizes returns the total size of each folder directly inside root,
// biggest first (at most 60), so a person can see where the space went and
// drill into the biggest one.
func FolderSizes(ctx context.Context, root string, progress ProgressFunc) ([]FolderSize, error) {
	if err := checkRoot(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	th := newThrottle("folders", progress)
	var out []FolderSize
	for _, e := range entries {
		if !e.IsDir() || !plain(e) || skipDirs[strings.ToLower(e.Name())] {
			continue
		}
		fsz := FolderSize{Path: filepath.Join(root, e.Name()), Name: e.Name()}
		werr := walkAll(ctx, fsz.Path, nil, func(p string, info fs.FileInfo) {
			fsz.Files++
			fsz.SizeBytes += info.Size()
			th.add(1, info.Size(), p)
		})
		if werr != nil {
			th.done()
			return out, werr
		}
		out = append(out, fsz)
	}
	th.done()
	sort.Slice(out, func(i, j int) bool {
		if out[i].SizeBytes != out[j].SizeBytes {
			return out[i].SizeBytes > out[j].SizeBytes
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 60 {
		out = out[:60]
	}
	if out == nil {
		out = []FolderSize{}
	}
	return out, nil
}
