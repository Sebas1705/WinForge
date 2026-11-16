package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A project's build output and installed dependencies take a lot of room and come back with one command
// (npm install, cargo build, dotnet build). They are listed only when the folder beside them proves what
// they are, and only for projects nobody has touched for a while.

// ProjectJunk is one such folder.
type ProjectJunk struct {
	Path        string    `json:"path"`
	Project     string    `json:"project"`     // the project folder's name
	ProjectPath string    `json:"projectPath"` // where the project is
	Kind        string    `json:"kind"`        // see junkKinds
	SizeBytes   int64     `json:"sizeBytes"`
	Files       int       `json:"files"`
	LastTouched time.Time `json:"lastTouched"` // the newest sign of work in the project, build output not counted
}

type junkKind struct {
	id      string
	dir     string   // the folder's name
	markers []string // one of these (names, "*" allowed) must sit beside it
	inside  []string // when set, one of these must be inside it
}

// junkKinds is the whole list. Adding a kind is adding an entry: nothing is guessed from a folder's name alone.
var junkKinds = []junkKind{
	{id: "node_modules", dir: "node_modules", markers: []string{"package.json"}},
	{id: "rust-target", dir: "target", markers: []string{"Cargo.toml"}, inside: []string{"CACHEDIR.TAG", ".rustc_info.json", "debug", "release"}},
	{id: "maven-target", dir: "target", markers: []string{"pom.xml"}},
	{id: "dotnet-obj", dir: "obj", markers: []string{"*.csproj", "*.fsproj", "*.vbproj"}},
	{id: "dotnet-bin", dir: "bin", markers: []string{"*.csproj", "*.fsproj", "*.vbproj"}},
	{id: "gradle-build", dir: "build", markers: []string{"build.gradle", "build.gradle.kts"}},
	{id: "gradle-cache", dir: ".gradle", markers: []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}},
}

// MinProjectAge is the least time a project must have been left alone before its output is listed or removed.
const MinProjectAge = 24 * time.Hour

const maxProjectDepth = 12

// neverEnter are folders the search does not go into: version-control data and the recycle bin.
var neverEnter = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "$recycle.bin": true, "system volume information": true,
}

func matchesAny(name string, patterns []string) bool {
	l := strings.ToLower(name)
	for _, p := range patterns {
		if ok, _ := filepath.Match(strings.ToLower(p), l); ok {
			return true
		}
	}
	return false
}

// junkIn returns the build-output folders of the project in dir, given its entries.
func junkIn(dir string, entries []fs.DirEntry) []junkKindHit {
	var files []string
	dirs := map[string]string{} // lower-case name -> real name
	for _, e := range entries {
		switch {
		case !plain(e):
		case e.IsDir():
			dirs[strings.ToLower(e.Name())] = e.Name()
		default:
			files = append(files, e.Name())
		}
	}
	var out []junkKindHit
	taken := map[string]bool{}
	for _, k := range junkKinds {
		real, ok := dirs[strings.ToLower(k.dir)]
		if !ok || taken[real] {
			continue
		}
		beside := false
		for _, f := range files {
			if matchesAny(f, k.markers) {
				beside = true
				break
			}
		}
		if !beside {
			continue
		}
		path := filepath.Join(dir, real)
		if len(k.inside) > 0 && !hasAny(path, k.inside) {
			continue
		}
		taken[real] = true
		out = append(out, junkKindHit{kind: k.id, path: path})
	}
	return out
}

type junkKindHit struct{ kind, path string }

func hasAny(dir string, names []string) bool {
	for _, n := range names {
		if _, err := os.Lstat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

// lastTouched is the newest sign of work in a project: what sits directly in its folder (the build output
// excluded) and what version control writes when someone commits, fetches or switches branch.
func lastTouched(dir string, entries []fs.DirEntry, hits []junkKindHit) time.Time {
	skip := map[string]bool{}
	for _, h := range hits {
		skip[strings.ToLower(filepath.Base(h.path))] = true
	}
	var newest time.Time
	see := func(info fs.FileInfo, err error) {
		if err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	for _, e := range entries {
		// .git is read through the few files that change when someone works; the folder itself also changes
		// when a tool merely looks around.
		if skip[strings.ToLower(e.Name())] || strings.EqualFold(e.Name(), ".git") {
			continue
		}
		info, err := e.Info()
		see(info, err)
	}
	for _, rel := range []string{".git/index", ".git/HEAD", ".git/FETCH_HEAD", ".git/logs/HEAD"} {
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
		see(info, err)
	}
	return newest
}

// FindProjectJunk looks for build output and dependency folders of projects left alone for at least minAge,
// under the given folders. Nothing is changed.
func FindProjectJunk(ctx context.Context, roots []string, minAge time.Duration, now time.Time, progress ProgressFunc) ([]ProjectJunk, error) {
	if minAge < MinProjectAge {
		minAge = MinProjectAge
	}
	if now.IsZero() {
		now = time.Now()
	}
	for _, r := range roots {
		if err := checkRoot(r); err != nil {
			return nil, err
		}
	}
	roots = dropNested(roots)
	th := newThrottle("projects", progress)

	var found []ProjectJunk
	var visit func(dir string, depth int) error
	visit = func(dir string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil // a folder that cannot be read is skipped, not an error
		}
		th.add(0, 0, dir)
		hits := junkIn(dir, entries)
		inHit := map[string]bool{}
		if len(hits) > 0 {
			touched := lastTouched(dir, entries, hits)
			if !touched.IsZero() && now.Sub(touched) >= minAge {
				for _, h := range hits {
					found = append(found, ProjectJunk{Path: h.path, Project: filepath.Base(dir), ProjectPath: dir, Kind: h.kind, LastTouched: touched})
				}
			}
			for _, h := range hits {
				inHit[strings.ToLower(filepath.Base(h.path))] = true
			}
		}
		if depth >= maxProjectDepth {
			return nil
		}
		for _, e := range entries {
			if !e.IsDir() || !plain(e) {
				continue
			}
			l := strings.ToLower(e.Name())
			if neverEnter[l] || l == "node_modules" || inHit[l] {
				continue
			}
			if err := visit(filepath.Join(dir, e.Name()), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := visit(r, 0); err != nil {
			return nil, err
		}
	}

	// Measuring is the slow part: many small files. A few folders at a time.
	err := parallel(len(found), func(i int) error {
		var size int64
		var n int
		_, err := walk(ctx, found[i].Path, func(p string, info fs.FileInfo) {
			size += info.Size()
			n++
			th.add(1, info.Size(), p)
		})
		found[i].SizeBytes, found[i].Files = size, n
		return err
	})
	th.done()
	if err != nil {
		return nil, err
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].SizeBytes != found[j].SizeBytes {
			return found[i].SizeBytes > found[j].SizeBytes
		}
		return found[i].Path < found[j].Path
	})
	if found == nil {
		found = []ProjectJunk{}
	}
	return found, nil
}

// ProjectResult is what removing project folders did.
type ProjectResult struct {
	FreedBytes int64    `json:"freedBytes"`
	Removed    int      `json:"removed"` // folders removed
	Files      int      `json:"files"`
	Failed     []string `json:"failed"`  // folders that could not be removed in full, or were refused
	Reasons    []string `json:"reasons"` // why, in the same order
}

// verifyProjectJunk re-proves, at the moment of removal, that a path is still what the search said: a build
// folder of a project, still marked by the file beside it, a real folder and not a link, outside every
// protected place, and in a project that has stayed quiet. It is the last gate before anything is deleted.
func verifyProjectJunk(env Env, path string, now time.Time) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("not an absolute clean path")
	}
	parent := filepath.Dir(path)
	if filepath.Dir(parent) == parent {
		return errors.New("a drive root's folder")
	}
	st, err := os.Lstat(path)
	if err != nil {
		return errors.New("it is gone")
	}
	if !st.IsDir() || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errors.New("not a plain folder")
	}
	for _, p := range protectedDirs(env) {
		if sameDir(path, p) || within(path, p) {
			return errors.New("a protected folder")
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return errors.New("its project cannot be read")
	}
	hits := junkIn(parent, entries)
	is := false
	for _, h := range hits {
		if sameDir(h.path, path) {
			is = true
		}
	}
	if !is {
		return errors.New("it is no longer a build folder of a project")
	}
	if touched := lastTouched(parent, entries, hits); touched.IsZero() || now.Sub(touched) < MinProjectAge {
		return errors.New("the project was touched recently")
	}
	return nil
}

// RemoveProjectJunk deletes the given folders, each after verifyProjectJunk. Links inside are removed as
// links and never followed, so nothing outside the folder is touched. It cannot be undone, which is why the
// caller asks first; every folder is independent.
func RemoveProjectJunk(ctx context.Context, env Env, paths []string, now time.Time, progress ProgressFunc) ProjectResult {
	if now.IsZero() {
		now = time.Now()
	}
	res := ProjectResult{Failed: []string{}, Reasons: []string{}}
	th := newThrottle("projects-clean", progress)
	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		if err := verifyProjectJunk(env, p, now); err != nil {
			res.Failed, res.Reasons = append(res.Failed, p), append(res.Reasons, err.Error())
			continue
		}
		freed, n, ok := removeTree(ctx, p, th)
		res.FreedBytes += freed
		res.Files += n
		if ok {
			res.Removed++
		} else {
			res.Failed, res.Reasons = append(res.Failed, p), append(res.Reasons, "some files could not be removed")
		}
	}
	th.done()
	return res
}

// removeTree deletes a folder and everything in it. A link is removed itself and never entered. It reports
// the bytes and files removed and whether the folder is gone.
func removeTree(ctx context.Context, root string, th *throttle) (freed int64, files int, gone bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, 0, false
	}
	clean := true
	for _, e := range entries {
		if ctx.Err() != nil {
			return freed, files, false
		}
		p := filepath.Join(root, e.Name())
		switch {
		case !plain(e):
			if !removeFile(p) && !removeDirLink(p) {
				clean = false
			}
		case e.IsDir():
			f, n, ok := removeTree(ctx, p, th)
			freed, files = freed+f, files+n
			if !ok {
				clean = false
			}
		default:
			info, ierr := e.Info()
			if removeFile(p) {
				files++
				if ierr == nil {
					freed += info.Size()
					th.add(1, info.Size(), p)
				}
			} else {
				clean = false
			}
		}
	}
	if !clean {
		return freed, files, false
	}
	if err := os.Remove(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return freed, files, false
	}
	return freed, files, true
}

// removeDirLink removes a link that points at a folder (a junction on Windows), which os.Remove handles
// as a link and not as the folder it points at.
func removeDirLink(p string) bool {
	err := os.Remove(p)
	return err == nil || errors.Is(err, os.ErrNotExist)
}
