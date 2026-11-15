package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

const day = 24 * time.Hour

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// touchAll sets the age of every entry directly inside dir (not the junk folders).
func age(t *testing.T, path string, old time.Duration) {
	t.Helper()
	mt := now.Add(-old)
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatal(err)
	}
}

// project lays out a project folder under root: marker files, a source file of the given age and junk files.
func project(t *testing.T, root, name string, markers []string, idle time.Duration) string {
	t.Helper()
	dir := filepath.Join(root, name)
	mkdirs(t, dir)
	for _, m := range markers {
		put(t, filepath.Join(dir, m), 20, idle)
	}
	put(t, filepath.Join(dir, "src", "main.rs"), 50, idle)
	age(t, filepath.Join(dir, "src"), idle)
	age(t, dir, idle)
	return dir
}

func find(t *testing.T, roots []string, minAge time.Duration) []storage.ProjectJunk {
	t.Helper()
	got, err := storage.FindProjectJunk(context.Background(), roots, minAge, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func byPath(list []storage.ProjectJunk) map[string]storage.ProjectJunk {
	m := map[string]storage.ProjectJunk{}
	for _, j := range list {
		m[j.Path] = j
	}
	return m
}

func TestOldProjectsGiveUpTheirDependenciesAndBuildOutput(t *testing.T) {
	root := t.TempDir()
	web := project(t, root, "old-site", []string{"package.json", "package-lock.json"}, 90*day)
	put(t, filepath.Join(web, "node_modules", "left-pad", "index.js"), 3000, 90*day)
	put(t, filepath.Join(web, "node_modules", ".bin", "tool"), 1000, 90*day)
	rust := project(t, root, "old-tool", []string{"Cargo.toml"}, 120*day)
	put(t, filepath.Join(rust, "target", "CACHEDIR.TAG"), 10, 120*day)
	put(t, filepath.Join(rust, "target", "debug", "app.exe"), 9000, 120*day)
	dotnet := project(t, root, "old-app", []string{"App.csproj"}, 200*day)
	put(t, filepath.Join(dotnet, "obj", "project.assets.json"), 400, 200*day)
	put(t, filepath.Join(dotnet, "bin", "Debug", "app.dll"), 800, 200*day)
	gradle := project(t, root, "old-android", []string{"build.gradle.kts", "settings.gradle.kts"}, 150*day)
	put(t, filepath.Join(gradle, "build", "outputs", "app.apk"), 7000, 150*day)
	put(t, filepath.Join(gradle, ".gradle", "8.5", "cache.bin"), 600, 150*day)
	maven := project(t, root, "old-java", []string{"pom.xml"}, 100*day)
	put(t, filepath.Join(maven, "target", "classes", "A.class"), 300, 100*day)

	got := byPath(find(t, []string{root}, 60*day))
	want := map[string]struct {
		kind string
		size int64
	}{
		filepath.Join(web, "node_modules"): {"node_modules", 4000},
		filepath.Join(rust, "target"):      {"rust-target", 9010},
		filepath.Join(dotnet, "obj"):       {"dotnet-obj", 400},
		filepath.Join(dotnet, "bin"):       {"dotnet-bin", 800},
		filepath.Join(gradle, "build"):     {"gradle-build", 7000},
		filepath.Join(gradle, ".gradle"):   {"gradle-cache", 600},
		filepath.Join(maven, "target"):     {"maven-target", 300},
	}
	if len(got) != len(want) {
		t.Fatalf("%d folders found, want %d: %v", len(got), len(want), got)
	}
	for path, w := range want {
		j, ok := got[path]
		if !ok || j.Kind != w.kind || j.SizeBytes != w.size {
			t.Errorf("%s: %+v, want %+v", path, j, w)
		}
		if j.Project != filepath.Base(filepath.Dir(path)) || j.ProjectPath != filepath.Dir(path) || j.Files == 0 || !j.LastTouched.Before(now.Add(-60*day)) {
			t.Errorf("%s: %+v", path, j)
		}
	}
}

func TestAFolderIsOnlyListedWhenSomethingBesideItProvesWhatItIs(t *testing.T) {
	root := t.TempDir()
	// no package.json: whatever this is, it is not known to be rebuilt by npm install
	a := project(t, root, "no-manifest", nil, 400*day)
	put(t, filepath.Join(a, "node_modules", "x", "y.js"), 100, 400*day)
	// a folder called target with no Cargo.toml or pom.xml
	b := project(t, root, "plain", []string{"README.md"}, 400*day)
	put(t, filepath.Join(b, "target", "important.doc"), 100, 400*day)
	// a Rust project whose target folder does not look like cargo made it
	c := project(t, root, "odd-rust", []string{"Cargo.toml"}, 400*day)
	put(t, filepath.Join(c, "target", "my-notes.txt"), 100, 400*day)
	// bin and obj with no project file
	d := project(t, root, "scripts", []string{"build.sh"}, 400*day)
	put(t, filepath.Join(d, "bin", "tool.sh"), 100, 400*day)
	// build with no gradle file
	e := project(t, root, "site", []string{"index.html"}, 400*day)
	put(t, filepath.Join(e, "build", "out.txt"), 100, 400*day)
	if got := find(t, []string{root}, 60*day); len(got) != 0 {
		t.Fatalf("nothing here is provably build output: %v", got)
	}
}

func TestRecentWorkKeepsAProjectOutOfTheList(t *testing.T) {
	root := t.TempDir()
	busy := project(t, root, "busy", []string{"package.json"}, 90*day)
	put(t, filepath.Join(busy, "node_modules", "a.js"), 100, 90*day)
	put(t, filepath.Join(busy, "notes.md"), 10, 2*day) // edited two days ago
	quiet := project(t, root, "quiet", []string{"package.json"}, 90*day)
	put(t, filepath.Join(quiet, "node_modules", "a.js"), 100, 90*day)

	got := find(t, []string{root}, 60*day)
	if len(got) != 1 || got[0].Project != "quiet" {
		t.Fatalf("%v", got)
	}
	// a lower limit brings the second one in, a higher one drops both
	if len(find(t, []string{root}, 1*day)) != 2 {
		t.Fatal("a short limit lists both")
	}
	if len(find(t, []string{root}, 365*day)) != 0 {
		t.Fatal("a long limit lists none")
	}
}

func TestVersionControlActivityCountsAsWork(t *testing.T) {
	root := t.TempDir()
	p := project(t, root, "committed-yesterday", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "a.js"), 100, 90*day)
	put(t, filepath.Join(p, ".git", "index"), 100, 1*day) // someone committed or switched branch yesterday
	if got := find(t, []string{root}, 60*day); len(got) != 0 {
		t.Fatalf("a project with a commit from yesterday is not old: %v", got)
	}
	q := project(t, root, "committed-long-ago", []string{"package.json"}, 90*day)
	put(t, filepath.Join(q, "node_modules", "a.js"), 100, 90*day)
	put(t, filepath.Join(q, ".git", "index"), 100, 90*day)
	if got := find(t, []string{root}, 60*day); len(got) != 1 || got[0].Project != "committed-long-ago" {
		t.Fatalf("%v", got)
	}
}

func TestTheLimitNeverDropsBelowADay(t *testing.T) {
	root := t.TempDir()
	p := project(t, root, "fresh", []string{"package.json"}, 3*time.Hour)
	put(t, filepath.Join(p, "node_modules", "a.js"), 100, 3*time.Hour)
	if got := find(t, []string{root}, 0); len(got) != 0 {
		t.Fatalf("a project touched this morning is never old: %v", got)
	}
}

func TestNestedPackagesAreFoundButNotWhatLiesInsideDependencies(t *testing.T) {
	root := t.TempDir()
	mono := project(t, root, "mono", []string{"package.json"}, 90*day)
	put(t, filepath.Join(mono, "node_modules", "dep", "package.json"), 20, 90*day)
	put(t, filepath.Join(mono, "node_modules", "dep", "node_modules", "deeper", "x.js"), 500, 90*day)
	pkg := project(t, filepath.Join(mono, "packages"), "ui", []string{"package.json"}, 90*day)
	put(t, filepath.Join(pkg, "node_modules", "y.js"), 200, 90*day)
	age(t, filepath.Join(mono, "packages"), 90*day)

	got := find(t, []string{root}, 60*day)
	paths := byPath(got)
	if len(got) != 2 || paths[filepath.Join(mono, "node_modules")].SizeBytes == 0 || paths[filepath.Join(pkg, "node_modules")].SizeBytes != 200 {
		t.Fatalf("each project's own node_modules once, never one inside another's: %v", got)
	}
	if _, nested := paths[filepath.Join(mono, "node_modules", "dep", "node_modules")]; nested {
		t.Fatal("a dependency's own node_modules is part of the outer one")
	}
}

func TestBiggestFirstAndTheSizeIsTheWholeFolder(t *testing.T) {
	root := t.TempDir()
	for i, size := range []int{100, 5000, 900} {
		p := project(t, root, "p"+string(rune('a'+i)), []string{"package.json"}, 90*day)
		put(t, filepath.Join(p, "node_modules", "a", "b.js"), size/2, 90*day)
		put(t, filepath.Join(p, "node_modules", "c.js"), size-size/2, 90*day)
	}
	got := find(t, []string{root}, 60*day)
	if len(got) != 3 || got[0].SizeBytes != 5000 || got[1].SizeBytes != 900 || got[2].SizeBytes != 100 || got[0].Files != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestLinksAreNeverFollowedWhileSearching(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	hidden := project(t, outside, "elsewhere", []string{"package.json"}, 90*day)
	put(t, filepath.Join(hidden, "node_modules", "a.js"), 100, 90*day)
	if err := os.Symlink(outside, filepath.Join(root, "link-to-outside")); err != nil {
		t.Skipf("cannot create links here: %v", err)
	}
	if got := find(t, []string{root}, 60*day); len(got) != 0 {
		t.Fatalf("the search followed a link out of the folder: %v", got)
	}
}

func TestTheSearchSkipsVersionControlFoldersAndNeedsRealFolders(t *testing.T) {
	root := t.TempDir()
	// something shaped like a project inside .git must not be entered
	hidden := project(t, filepath.Join(root, ".git"), "modules", []string{"package.json"}, 90*day)
	put(t, filepath.Join(hidden, "node_modules", "a.js"), 10, 90*day)
	if got := find(t, []string{root}, 60*day); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if _, err := storage.FindProjectJunk(context.Background(), []string{filepath.Join(root, "missing")}, 60*day, now, nil); err == nil {
		t.Fatal("a folder that is not there must say so")
	}
	if _, err := storage.FindProjectJunk(context.Background(), []string{"relative/dir"}, 60*day, now, nil); err == nil {
		t.Fatal("a relative folder must be refused")
	}
	file := filepath.Join(root, "f.txt")
	put(t, file, 1, 0)
	if _, err := storage.FindProjectJunk(context.Background(), []string{file}, 60*day, now, nil); err == nil {
		t.Fatal("a file is not a folder")
	}
}

func TestNestedRootsAreSearchedOnce(t *testing.T) {
	root := t.TempDir()
	p := project(t, filepath.Join(root, "work"), "a", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "x.js"), 10, 90*day)
	got := find(t, []string{root, filepath.Join(root, "work")}, 60*day)
	if len(got) != 1 {
		t.Fatalf("listed twice: %v", got)
	}
}

func TestTheSearchCanBeStopped(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		p := project(t, root, "p"+string(rune('a'+i)), []string{"package.json"}, 90*day)
		put(t, filepath.Join(p, "node_modules", "x.js"), 10, 90*day)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := storage.FindProjectJunk(ctx, []string{root}, 60*day, now, nil); err == nil || got != nil {
		t.Fatalf("a stopped search returns nothing and says so: %v %v", got, err)
	}
}

func TestProgressIsReported(t *testing.T) {
	root := t.TempDir()
	p := project(t, root, "p", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "x.js"), 10, 90*day)
	var mu sync.Mutex
	var last storage.Progress
	if _, err := storage.FindProjectJunk(context.Background(), []string{root}, 60*day, now, func(pr storage.Progress) {
		mu.Lock()
		last = pr
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	if last.Task != "projects" || last.Files != 1 || last.Bytes != 10 {
		t.Fatalf("%+v", last)
	}
}

// ---- removal ----

func remove(t *testing.T, e storage.Env, paths ...string) storage.ProjectResult {
	t.Helper()
	return storage.RemoveProjectJunk(context.Background(), e, paths, now, nil)
}

func TestRemovingAProjectFolderFreesItAndLeavesTheProject(t *testing.T) {
	e := fakeEnv(t)
	root := filepath.Join(e.Home, "code")
	p := project(t, root, "old", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "a", "b.js"), 3000, 90*day)
	put(t, filepath.Join(p, "node_modules", "c.js"), 1000, 90*day)
	// a read-only file, as npm leaves some
	ro := filepath.Join(p, "node_modules", "readonly.txt")
	put(t, ro, 500, 90*day)
	if err := os.Chmod(ro, 0o444); err != nil {
		t.Fatal(err)
	}
	listed := find(t, []string{root}, 60*day)
	if len(listed) != 1 {
		t.Fatal("setup")
	}
	res := remove(t, e, listed[0].Path)
	if res.Removed != 1 || res.FreedBytes != 4500 || res.Files != 3 || len(res.Failed) != 0 {
		t.Fatalf("%+v", res)
	}
	if exists(listed[0].Path) {
		t.Fatal("the folder is still there")
	}
	for _, keep := range []string{filepath.Join(p, "package.json"), filepath.Join(p, "src", "main.rs"), p} {
		if !exists(keep) {
			t.Errorf("%s must stay", keep)
		}
	}
}

func TestLinksInsideAFolderAreRemovedAsLinksAndNeverFollowed(t *testing.T) {
	e := fakeEnv(t)
	outside := t.TempDir()
	keepFile := filepath.Join(outside, "precious.txt")
	put(t, keepFile, 100, 0)
	root := filepath.Join(e.Home, "code")
	p := project(t, root, "linked", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "real.js"), 50, 90*day)
	if err := os.Symlink(outside, filepath.Join(p, "node_modules", "link-to-dir")); err != nil {
		t.Skipf("cannot create links here: %v", err)
	}
	if err := os.Symlink(keepFile, filepath.Join(p, "node_modules", "link-to-file")); err != nil {
		t.Skipf("cannot create links here: %v", err)
	}
	res := remove(t, e, filepath.Join(p, "node_modules"))
	if res.Removed != 1 || len(res.Failed) != 0 {
		t.Fatalf("%+v", res)
	}
	if !exists(keepFile) || !exists(outside) {
		t.Fatal("removing a folder followed a link and deleted what it points at")
	}
	if exists(filepath.Join(p, "node_modules")) {
		t.Fatal("the folder is still there")
	}
}

func TestNothingIsRemovedUnlessItIsStillProvablyBuildOutput(t *testing.T) {
	e := fakeEnv(t)
	root := filepath.Join(e.Home, "code")

	// a path that was never listed: a folder with a name that looks right but no project beside it
	stray := project(t, root, "stray", []string{"README.md"}, 90*day)
	put(t, filepath.Join(stray, "node_modules", "a.js"), 10, 90*day)

	// a project whose manifest was removed since the search
	gone := project(t, root, "manifest-removed", []string{"package.json"}, 90*day)
	put(t, filepath.Join(gone, "node_modules", "a.js"), 10, 90*day)
	if err := os.Remove(filepath.Join(gone, "package.json")); err != nil {
		t.Fatal(err)
	}

	// a project that was edited after the search
	busy := project(t, root, "busy-now", []string{"package.json"}, 90*day)
	put(t, filepath.Join(busy, "node_modules", "a.js"), 10, 90*day)
	put(t, filepath.Join(busy, "new-file.txt"), 1, 1*time.Hour)

	// the project folder itself, and a source folder
	real := project(t, root, "real", []string{"package.json"}, 90*day)
	put(t, filepath.Join(real, "node_modules", "a.js"), 10, 90*day)

	paths := []string{
		filepath.Join(stray, "node_modules"), filepath.Join(gone, "node_modules"), filepath.Join(busy, "node_modules"),
		real, filepath.Join(real, "src"), filepath.Join(real, "package.json"), filepath.Join(root, "missing"),
		e.Home, filepath.Join(e.Home, "Documents"), root, "relative/path", "", filepath.Dir(e.SystemDrive),
	}
	res := remove(t, e, paths...)
	if res.Removed != 0 || res.FreedBytes != 0 || len(res.Failed) != len(paths) || len(res.Reasons) != len(paths) {
		t.Fatalf("%+v", res)
	}
	for _, p := range []string{stray, gone, busy, real, filepath.Join(real, "node_modules"), filepath.Join(real, "src"), filepath.Join(busy, "node_modules")} {
		if !exists(p) {
			t.Errorf("%s was removed", p)
		}
	}
	for i, why := range res.Reasons {
		if why == "" {
			t.Errorf("%q was refused without a reason", paths[i])
		}
	}
}

func TestALinkIsNotAFolderToRemove(t *testing.T) {
	e := fakeEnv(t)
	root := filepath.Join(e.Home, "code")
	target := t.TempDir()
	put(t, filepath.Join(target, "x.txt"), 10, 90*day)
	p := project(t, root, "links", []string{"package.json"}, 90*day)
	if err := os.Symlink(target, filepath.Join(p, "node_modules")); err != nil {
		t.Skipf("cannot create links here: %v", err)
	}
	res := remove(t, e, filepath.Join(p, "node_modules"))
	if res.Removed != 0 || len(res.Failed) != 1 || !exists(filepath.Join(target, "x.txt")) {
		t.Fatalf("%+v", res)
	}
}

func TestProtectedPlacesAreNeverRemoved(t *testing.T) {
	e := fakeEnv(t)
	// a project in the Documents folder is the person's own place: even a real node_modules is refused
	p := project(t, filepath.Join(e.Home, "Documents"), "work", []string{"package.json"}, 90*day)
	put(t, filepath.Join(p, "node_modules", "a.js"), 10, 90*day)
	// Documents itself is protected, but a build folder inside it is the person's project and is found.
	if got := find(t, []string{filepath.Join(e.Home, "Documents")}, 60*day); len(got) != 1 {
		t.Fatalf("a project inside Documents is a project: %v", got)
	}
	res := remove(t, e, filepath.Join(e.Home, "Documents"), e.LocalAppData, e.WinDir)
	if res.Removed != 0 || len(res.Failed) != 3 {
		t.Fatalf("%+v", res)
	}
}

func TestEachFolderIsIndependentAndTheRunCanBeStopped(t *testing.T) {
	e := fakeEnv(t)
	root := filepath.Join(e.Home, "code")
	var good []string
	for _, n := range []string{"a", "b", "c"} {
		p := project(t, root, n, []string{"package.json"}, 90*day)
		put(t, filepath.Join(p, "node_modules", "x.js"), 100, 90*day)
		good = append(good, filepath.Join(p, "node_modules"))
	}
	res := remove(t, e, good[0], filepath.Join(root, "nope"), good[1], good[2])
	if res.Removed != 3 || len(res.Failed) != 1 || res.FreedBytes != 300 {
		t.Fatalf("one refusal must not stop the others: %+v", res)
	}
	q := project(t, root, "d", []string{"package.json"}, 90*day)
	put(t, filepath.Join(q, "node_modules", "x.js"), 100, 90*day)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res = storage.RemoveProjectJunk(ctx, e, []string{filepath.Join(q, "node_modules")}, now, nil)
	if res.Removed != 0 || !exists(filepath.Join(q, "node_modules", "x.js")) {
		t.Fatalf("a stopped run removes nothing: %+v", res)
	}
}

func TestTheResultNeverHasNullLists(t *testing.T) {
	res := storage.RemoveProjectJunk(context.Background(), fakeEnv(t), nil, now, nil)
	if res.Failed == nil || res.Reasons == nil {
		t.Fatalf("%+v", res)
	}
	if got, err := storage.FindProjectJunk(context.Background(), []string{t.TempDir()}, 60*day, now, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("an empty result is an empty list, not null: %v %v", got, err)
	}
}
