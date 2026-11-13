package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DupFile is one copy of a duplicated file.
type DupFile struct {
	Path     string    `json:"path"`
	Modified time.Time `json:"modified"`
}

// DupGroup is a set of files with identical content. Files are listed oldest
// first, which is the copy to keep by default.
type DupGroup struct {
	ID        string    `json:"id"`
	SizeBytes int64     `json:"sizeBytes"`
	Files     []DupFile `json:"files"`
}

// Waste is how much space removing all but one copy frees.
func (g DupGroup) Waste() int64 { return g.SizeBytes * int64(len(g.Files)-1) }

// dupSkip are folders full of files that are meant to repeat: dependencies,
// version-control objects, system folders. Listing them as duplicates would
// only invite breaking a project.
var dupSkip = map[string]bool{
	"node_modules":              true,
	".git":                      true,
	"$recycle.bin":              true,
	"system volume information": true,
	"winsxs":                    true,
}

const (
	partialHash = 64 << 10
	maxGroups   = 500
)

type cand struct {
	path string
	size int64
	mod  time.Time
	info fs.FileInfo
}

// FindDuplicates finds files at least minBytes big with the same content in the
// given folders. It narrows by size, then by a hash of the first 64 KB, and only
// then reads whole files, so unlike files cost almost nothing. Hard links to the
// same data are not duplicates: removing one frees nothing.
func FindDuplicates(ctx context.Context, roots []string, minBytes int64, progress ProgressFunc) ([]DupGroup, error) {
	if minBytes < 1 {
		minBytes = 1
	}
	var clean []string
	for _, r := range roots {
		if err := checkRoot(r); err != nil {
			return nil, err
		}
		clean = append(clean, filepath.Clean(r))
	}
	clean = dropNested(clean)

	th := newThrottle("dups", progress)
	bySize := map[int64][]cand{}
	seen := map[string]bool{}
	for _, r := range clean {
		err := walkAll(ctx, r, func(n string) bool { return dupSkip[n] }, func(p string, info fs.FileInfo) {
			th.add(1, 0, p)
			key := strings.ToLower(p)
			if info.Size() < minBytes || seen[key] {
				return
			}
			seen[key] = true
			bySize[info.Size()] = append(bySize[info.Size()], cand{p, info.Size(), info.ModTime(), info})
		})
		if err != nil {
			return nil, err
		}
	}

	var groups []DupGroup
	for size, list := range bySize {
		if len(list) < 2 {
			continue
		}
		byPartial := map[string][]cand{}
		for _, c := range list {
			h, err := hashFile(ctx, c.path, partialHash)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			byPartial[h] = append(byPartial[h], c)
		}
		for _, same := range byPartial {
			if len(same) < 2 {
				continue
			}
			byFull := map[string][]cand{}
			for _, c := range same {
				h, err := hashFile(ctx, c.path, -1)
				if err != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					continue
				}
				th.add(0, c.size, c.path)
				byFull[h] = append(byFull[h], c)
			}
			for h, copies := range byFull {
				copies = distinctData(copies)
				if len(copies) < 2 {
					continue
				}
				sort.Slice(copies, func(i, j int) bool {
					if !copies[i].mod.Equal(copies[j].mod) {
						return copies[i].mod.Before(copies[j].mod)
					}
					return copies[i].path < copies[j].path
				})
				g := DupGroup{ID: h[:12], SizeBytes: size}
				for _, c := range copies {
					g.Files = append(g.Files, DupFile{Path: c.path, Modified: c.mod})
				}
				groups = append(groups, g)
			}
		}
	}
	th.done()
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Waste() != groups[j].Waste() {
			return groups[i].Waste() > groups[j].Waste()
		}
		return groups[i].ID < groups[j].ID
	})
	if len(groups) > maxGroups {
		groups = groups[:maxGroups]
	}
	if groups == nil {
		groups = []DupGroup{}
	}
	return groups, nil
}

// distinctData drops copies that are hard links to the same data as an earlier one.
func distinctData(in []cand) []cand {
	var out []cand
next:
	for _, c := range in {
		for _, k := range out {
			if os.SameFile(k.info, c.info) {
				continue next
			}
		}
		out = append(out, c)
	}
	return out
}

// dropNested removes folders that lie inside another listed folder, so the same
// files are not walked twice.
func dropNested(in []string) []string {
	var out []string
	for i, a := range in {
		nested := false
		for j, b := range in {
			if i != j && (within(b, a) || (strings.EqualFold(a, b) && j < i)) {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, a)
		}
	}
	return out
}

// hashFile returns the SHA-256 of a file, or of its first n bytes when n >= 0.
func hashFile(ctx context.Context, path string, n int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	var src io.Reader = f
	if n >= 0 {
		src = io.LimitReader(f, n)
	}
	buf := make([]byte, 256<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		k, rerr := src.Read(buf)
		if k > 0 {
			h.Write(buf[:k])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
