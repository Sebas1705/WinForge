package uninstall

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// Kind is what a candidate is.
type Kind string

const (
	KindFolder   Kind = "folder"
	KindShortcut Kind = "shortcut"
)

// MaxCandidates stops a search that matches an absurd amount; the list is for a person to read.
const MaxCandidates = 200

// Candidate is a folder or shortcut that may be left over from an app.
type Candidate struct {
	Path      string    `json:"path"`
	Kind      Kind      `json:"kind"`
	Place     PlaceID   `json:"place"`
	SizeBytes int64     `json:"sizeBytes"`
	Files     int       `json:"files"`
	Modified  time.Time `json:"modified"`
	Why       Why       `json:"why"`
}

// Searched is a place the search read, so the page can say where it looked.
type Searched struct {
	Place PlaceID `json:"place"`
	Path  string  `json:"path"`
}

// Finder searches and removes. It holds no state: what a search listed is remembered by the caller.
type Finder struct {
	Sys System
	Env Env
}

// reasons a removal is refused, as keys the interface translates.
const (
	ReasonMissing   = "missing"   // it is already gone
	ReasonLink      = "link"      // not a plain folder or file: a link or junction
	ReasonProtected = "protected" // a protected folder, or one that contains one
	ReasonPlace     = "place"     // not directly inside one of the fixed places
	ReasonChanged   = "changed"   // no longer matches the app, or belongs to another installed app
	ReasonNotMoved  = "notmoved"  // the Recycle Bin could not take it: in use, or no rights
)

// Failure is one path that was not moved, and why.
type Failure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Result is what a removal did.
type Result struct {
	Moved      int       `json:"moved"`
	FreedBytes int64     `json:"freedBytes"`
	Failed     []Failure `json:"failed"`
}

// others names the other things that must never be taken for the app: the catalog apps still installed and
// whatever Windows lists as installed.
type guard struct {
	others    []App
	installed []string
}

func (g guard) claims(name string) bool {
	for _, o := range g.others {
		if _, ok := Match(o, name); ok {
			return true
		}
	}
	for _, n := range g.installed {
		if same(n, name) {
			return true
		}
	}
	return false
}

func (f Finder) guard(others []App) guard {
	g := guard{others: others}
	for _, r := range f.Sys.Installed() {
		g.installed = append(g.installed, r.Name)
	}
	return g
}

// check is the gate in front of everything: a path is accepted only when it is a plain folder (or, for a
// shortcut, a plain .lnk file) directly inside a fixed place, outside every protected folder.
func (f Finder) check(path string, kind Kind) (Place, Entry, error) {
	var none Place
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return none, Entry{}, errors.New(ReasonPlace)
	}
	if filepath.Dir(path) == path {
		return none, Entry{}, errors.New(ReasonProtected)
	}
	for _, p := range f.Env.protected() {
		if sameDir(path, p) || storage.Within(path, p) {
			return none, Entry{}, errors.New(ReasonProtected)
		}
	}
	for _, p := range f.Env.untouchable() {
		if sameDir(path, p) || storage.Within(p, path) {
			return none, Entry{}, errors.New(ReasonProtected)
		}
	}
	place, ok := f.placeOf(path, kind)
	if !ok {
		return none, Entry{}, errors.New(ReasonPlace)
	}
	if kind == KindFolder {
		if err := storage.SafeRoot(f.Env.storageEnv(), path); err != nil {
			return none, Entry{}, errors.New(ReasonProtected)
		}
	}
	if parent := filepath.Dir(path); !sameDir(parent, place.Dir) {
		// "Publisher\Name": the publisher folder must itself be a plain folder, or the path would lead through a link.
		if pe, err := f.Sys.Lstat(parent); err != nil || pe.Link || !pe.Dir {
			return none, Entry{}, errors.New(ReasonLink)
		}
	}
	e, err := f.Sys.Lstat(path)
	if err != nil {
		return none, Entry{}, errors.New(ReasonMissing)
	}
	if e.Link || (kind == KindFolder) != e.Dir {
		return none, Entry{}, errors.New(ReasonLink)
	}
	return place, e, nil
}

// placeOf finds the place a path belongs to: its parent is the place (a folder or shortcut directly
// inside), or its grandparent is a place that allows "Publisher\Name" and the parent is a real folder.
func (f Finder) placeOf(path string, kind Kind) (Place, bool) {
	parent := filepath.Dir(path)
	for _, p := range f.Env.Places() {
		switch {
		case sameDir(parent, p.Dir):
			if (kind == KindFolder && p.Folders) || (kind == KindShortcut && p.Shortcuts && strings.EqualFold(filepath.Ext(path), ".lnk")) {
				return p, true
			}
		case kind == KindFolder && p.Nested && sameDir(filepath.Dir(parent), p.Dir):
			return p, true
		}
	}
	return Place{}, false
}

// Find lists what the app may have left behind. others are the catalog apps that are still installed: a
// folder that fits one of them is never listed. Nothing is changed.
func (f Finder) Find(ctx context.Context, app App, others []App) ([]Candidate, []Searched, error) {
	g := f.guard(others)
	var found []Candidate
	searched := []Searched{}
	seen := map[string]bool{}
	offer := func(pl Place, path string, kind Kind, why Why) {
		if len(found) >= MaxCandidates || seen[strings.ToLower(path)] {
			return
		}
		if _, _, err := f.check(path, kind); err != nil {
			return
		}
		seen[strings.ToLower(path)] = true
		found = append(found, Candidate{Path: path, Kind: kind, Place: pl.ID, Why: why})
	}
	for _, pl := range f.Env.Places() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		entries, err := f.Sys.ReadDir(pl.Dir)
		if err != nil {
			continue
		}
		searched = append(searched, Searched{Place: pl.ID, Path: pl.Dir})
		for _, e := range entries {
			if e.Link || strings.ContainsAny(e.Name, `\/`) {
				continue
			}
			path := filepath.Join(pl.Dir, e.Name)
			switch {
			case e.Dir && pl.Folders:
				if !IsSystemName(e.Name) {
					if why, ok := Match(app, e.Name); ok && !g.claims(e.Name) {
						offer(pl, path, KindFolder, why)
					}
				}
				if pl.Nested {
					f.nested(pl, app, g, path, e.Name, offer)
				}
			case !e.Dir && pl.Shortcuts:
				if n, ok := shortcutName(e.Name); ok && !IsSystemName(n) {
					if why, ok := Match(app, n); ok && !g.claims(n) {
						offer(pl, path, KindShortcut, why)
					}
				}
			}
		}
	}
	for i := range found {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		c := &found[i]
		st, _ := f.Sys.Lstat(c.Path)
		c.Modified = st.ModTime
		if c.Kind == KindShortcut {
			c.SizeBytes, c.Files = st.Size, 1
			continue
		}
		var newest time.Time
		c.SizeBytes, c.Files, newest = f.Sys.Measure(ctx, c.Path)
		if newest.After(c.Modified) {
			c.Modified = newest
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].Path < found[j].Path })
	if found == nil {
		found = []Candidate{}
	}
	return found, searched, nil
}

// nested looks one level inside a folder named like the app's publisher.
func (f Finder) nested(pl Place, app App, g guard, parentPath, parentName string, offer func(Place, string, Kind, Why)) {
	if app.Publisher == "" || protectedVendor(parentName) {
		return
	}
	if !same(app.Publisher, parentName) && !same(shortPublisher(app.Publisher), parentName) {
		return
	}
	kids, err := f.Sys.ReadDir(parentPath)
	if err != nil {
		return
	}
	for _, k := range kids {
		if !k.Dir || k.Link || strings.ContainsAny(k.Name, `\/`) || IsSystemName(k.Name) {
			continue
		}
		if why, ok := MatchNested(app, parentName, k.Name); ok && !g.claims(k.Name) {
			offer(pl, filepath.Join(parentPath, k.Name), KindFolder, why)
		}
	}
}

// Remove moves the given candidates to the Recycle Bin. Each one is proved again first: still inside a
// fixed place, outside every protected folder, a plain folder or .lnk (never a link), still named like the
// app and not claimed by another installed app. What fails is reported per path and left where it was.
func (f Finder) Remove(ctx context.Context, app App, others []App, items []Candidate) Result {
	res := Result{Failed: []Failure{}}
	g := f.guard(others)
	var folders, files []string
	for _, c := range items {
		if ctx.Err() != nil {
			res.Failed = append(res.Failed, Failure{c.Path, ReasonNotMoved})
			continue
		}
		if err := f.prove(app, g, c); err != nil {
			res.Failed = append(res.Failed, Failure{c.Path, err.Error()})
			continue
		}
		if c.Kind == KindFolder {
			folders = append(folders, c.Path)
		} else {
			files = append(files, c.Path)
		}
	}
	for _, batch := range []struct {
		paths []string
		move  func(context.Context, []string) (int, int64, []string)
	}{{folders, f.Sys.RecycleFolders}, {files, f.Sys.RecycleFiles}} {
		if len(batch.paths) == 0 {
			continue
		}
		moved, freed, failed := batch.move(ctx, batch.paths)
		res.Moved += moved
		res.FreedBytes += freed
		for _, p := range failed {
			res.Failed = append(res.Failed, Failure{p, ReasonNotMoved})
		}
	}
	return res
}

func (f Finder) prove(app App, g guard, c Candidate) error {
	if c.Kind != KindFolder && c.Kind != KindShortcut {
		return errors.New(ReasonPlace)
	}
	if _, _, err := f.check(c.Path, c.Kind); err != nil {
		return err
	}
	base := filepath.Base(c.Path)
	name := base
	if c.Kind == KindShortcut {
		n, ok := shortcutName(base)
		if !ok {
			return errors.New(ReasonPlace)
		}
		name = n
	}
	if IsSystemName(name) || g.claims(name) {
		return errors.New(ReasonChanged)
	}
	if _, ok := Match(app, name); ok {
		return nil
	}
	if c.Kind == KindFolder {
		if _, ok := MatchNested(app, filepath.Base(filepath.Dir(c.Path)), name); ok {
			return nil
		}
	}
	return errors.New(ReasonChanged)
}

// InstallSize is the size Windows records for an installed app, found through the catalog's registry
// patterns for it; 0 when no entry matches or the installer recorded none.
func InstallSize(patterns []string, regs []RegApp) int64 {
	var res []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile("(?i)" + p); err == nil {
			res = append(res, re)
		}
	}
	var best int64
	for _, r := range regs {
		if r.SizeKB <= best {
			continue
		}
		for _, re := range res {
			if re.MatchString(r.Name) {
				best = r.SizeKB
				break
			}
		}
	}
	return best * 1024
}
