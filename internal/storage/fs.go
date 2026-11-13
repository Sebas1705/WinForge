package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// expand turns a pattern whose "*" segments stand for any one folder name into
// the folders that exist. The pattern is split by hand rather than handed to
// filepath.Glob, because a profile name containing "[" would be read as a
// character class.
func expand(pattern string) []string {
	if pattern == "" {
		return nil
	}
	vol := filepath.VolumeName(pattern)
	rest := strings.TrimPrefix(pattern, vol)
	sep := string(filepath.Separator)
	abs := strings.HasPrefix(rest, sep)
	segs := strings.Split(strings.Trim(rest, sep), sep)
	base := vol
	if abs {
		base += sep
	}
	bases := []string{base}
	for _, seg := range segs {
		var next []string
		for _, b := range bases {
			if seg == "*" {
				entries, err := os.ReadDir(b)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if e.IsDir() && e.Type()&os.ModeSymlink == 0 {
						next = append(next, filepath.Join(b, e.Name()))
					}
				}
				continue
			}
			next = append(next, filepath.Join(b, seg))
		}
		bases = next
	}
	var out []string
	for _, b := range bases {
		if st, err := os.Lstat(b); err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
			out = append(out, b)
		}
	}
	return out
}

// protectedDirs are folders a cleanup root may never be, nor contain.
func protectedDirs(e Env) []string {
	var p []string
	add := func(parts ...string) {
		if joined := under(parts[0], parts[1:]...); joined != "" {
			p = append(p, joined)
		}
	}
	for _, base := range []string{e.Home, e.AppData, e.LocalAppData, e.ProgramData, e.WinDir} {
		if base != "" {
			p = append(p, filepath.Clean(base))
		}
	}
	if e.SystemDrive != "" {
		p = append(p, filepath.Clean(e.SystemDrive+string(filepath.Separator)))
	}
	for _, name := range []string{"Documents", "Desktop", "Downloads", "Pictures", "Videos", "Music", "OneDrive"} {
		add(e.Home, name)
	}
	return p
}

// safeRoot says whether a folder may have its contents removed: it must be an
// absolute, clean path that is not a drive root, a protected folder, or a
// folder that contains one (a TEMP variable pointing at the profile folder
// would otherwise empty it).
func safeRoot(e Env, dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return errors.New("not an absolute clean path")
	}
	if filepath.Dir(dir) == dir {
		return errors.New("a drive root")
	}
	for _, p := range protectedDirs(e) {
		if sameDir(dir, p) {
			return errors.New("a protected folder")
		}
		if within(dir, p) {
			return errors.New("contains a protected folder")
		}
	}
	return nil
}

func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// tempOK accepts the TEMP folder only when it looks like one: Windows keeps it
// at ...\AppData\Local\Temp, and people move it to a folder named Temp or Tmp.
func tempOK(e Env, dir string) bool {
	if e.LocalAppData != "" && sameDir(dir, filepath.Join(e.LocalAppData, "Temp")) {
		return true
	}
	last := strings.ToLower(filepath.Base(dir))
	return last == "temp" || last == "tmp"
}

// roots resolves a category's folders on this PC, dropping any that fail the
// safety check. refused lists what was dropped and why, for the caller to show.
func roots(e Env, s Spec) (ok []string, refused []string) {
	if s.Dirs == nil {
		return nil, nil
	}
	for _, pattern := range s.Dirs(e) {
		for _, d := range expand(pattern) {
			if s.ID == "temp-user" && !tempOK(e, d) {
				refused = append(refused, d+": TEMP does not point at a temporary folder")
				continue
			}
			if err := safeRoot(e, d); err != nil {
				refused = append(refused, d+": "+err.Error())
				continue
			}
			ok = append(ok, d)
		}
	}
	sort.Strings(ok)
	return ok, refused
}

// accept applies a category's file-name patterns and age limit.
func accept(s Spec, now time.Time, name string, modTime time.Time) bool {
	if s.MinAge > 0 && now.Sub(modTime) < s.MinAge {
		return false
	}
	if len(s.Files) == 0 {
		return true
	}
	l := strings.ToLower(name)
	for _, pat := range s.Files {
		if ok, _ := path.Match(strings.ToLower(pat), l); ok {
			return true
		}
	}
	return false
}

// plain reports whether a directory entry is an ordinary file or folder.
// Symbolic links, junctions and devices are skipped everywhere, so nothing
// ever leads out of the folder being worked on.
func plain(d fs.DirEntry) bool {
	return d.Type()&(os.ModeSymlink|os.ModeIrregular|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket) == 0
}

// walk visits the regular files under root. skipped counts what could not be read.
func walk(ctx context.Context, root string, visit func(path string, info fs.FileInfo)) (skipped int, err error) {
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if werr != nil {
			skipped++
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !plain(d) {
			skipped++
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			skipped++
			return nil
		}
		visit(p, info)
		return nil
	})
	return skipped, err
}
