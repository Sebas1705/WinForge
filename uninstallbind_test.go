package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/uninstall"
)

// binRunner stands in for PowerShell and the Recycle Bin: it removes whatever the test says it takes.
type binRunner struct {
	mu    sync.Mutex
	calls int
	take  func()
}

func (r *binRunner) Run(context.Context, string, ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.take != nil {
		r.take()
	}
	return nil, nil
}

// mklinkJunction makes a directory junction, which needs no privilege on Windows.
func mklinkJunction(link, target string) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("junctions only exist on Windows")
	}
	return exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
}

type uninstRig struct {
	a         *App
	env       uninstall.Env
	bin       *binRunner
	installed map[string]bool
	plans     []install.Plan
	startErr  error
	regs      []uninstall.RegApp
}

// uninstApp is an App over a temporary PC: real folders in a temp dir, a fake Recycle Bin, a fake scan of
// what is installed and a fake runner. Nothing is uninstalled or deleted for real.
func uninstApp(t *testing.T) *uninstRig {
	t.Helper()
	a := testApp(t)
	a.cat.Apps["git"].Publisher = "The Git Development Community"
	a.cat.Apps["git"].Detect.Registry = []string{`^Git version`}
	a.cat.Apps["vlc"].Publisher = "VideoLAN"
	a.cat.Apps["gitlfs"] = &catalog.App{ID: "gitlfs", Name: "Git LFS", Winget: "GitHub.GitLFS", Publisher: "GitHub, Inc."}
	a.cat.Apps["foo"] = &catalog.App{ID: "foo", Name: "Foo", Winget: "Acme.Foo", Publisher: "Acme"}
	a.cat.Apps["foopro"] = &catalog.App{ID: "foopro", Name: "Foo Pro", Winget: "Acme.Foo", Publisher: "Acme"}

	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	env := uninstall.Env{
		Home: home, AppData: filepath.Join(home, "AppData", "Roaming"), LocalAppData: filepath.Join(home, "AppData", "Local"),
		ProgramData: filepath.Join(root, "ProgramData"), ProgramFiles: filepath.Join(root, "Program Files"),
		ProgramFilesX86: filepath.Join(root, "Program Files (x86)"), WinDir: filepath.Join(root, "Windows"), SystemDrive: root,
		Public: filepath.Join(root, "Users", "Public"),
	}
	for _, d := range []string{env.AppData, env.LocalAppData, env.ProgramData, env.ProgramFiles, env.ProgramFilesX86, env.WinDir,
		filepath.Join(home, "Desktop"), filepath.Join(home, "Documents")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r := &uninstRig{a: a, env: env, bin: &binRunner{}, installed: map[string]bool{}}
	a.runner = r.bin
	a.uninst.env = &env
	a.uninst.sys = uninstall.NewSystemWithPrograms(r.bin, func() []uninstall.RegApp { return r.regs })
	a.uninst.detect = func() (map[string]system.Installed, error) {
		out := map[string]system.Installed{}
		for id := range r.installed {
			out[id] = system.Installed{ID: id, Version: "1.0", Sources: []string{"winget"}}
		}
		return out, nil
	}
	a.uninst.start = func(p install.Plan, title, _ string) error {
		if r.startErr != nil {
			return r.startErr
		}
		r.plans = append(r.plans, p)
		return nil
	}
	return r
}

func (r *uninstRig) setInstalled(ids ...string) {
	r.installed = map[string]bool{}
	for _, id := range ids {
		r.installed[id] = true
	}
	r.a.mu.Lock()
	r.a.installed = map[string]system.Installed{}
	for _, id := range ids {
		r.a.installed[id] = system.Installed{ID: id}
	}
	r.a.mu.Unlock()
}

func TestUninstallListShowsInstalledCatalogAppsWithSizes(t *testing.T) {
	r := uninstApp(t)
	r.setInstalled("vlc", "git", "7zip")
	r.regs = []uninstall.RegApp{{Name: "Git version 2.5", SizeKB: 300}, {Name: "Unrelated", SizeKB: 99999}}
	got, err := r.a.UninstallList()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, x := range got.Apps {
		names = append(names, x.Name)
	}
	if strings.Join(names, ",") != "7-Zip,Git,VLC" {
		t.Fatalf("sorted by name, only installed: %v", names)
	}
	if got.Apps[0].Admin != true || got.Apps[1].Version != "1.0" || got.Apps[1].SizeBytes != 300*1024 || got.Apps[2].SizeBytes != 0 || got.Apps[1].Publisher == "" {
		t.Fatalf("%+v", got.Apps)
	}
	if len(got.Removed) != 0 || got.Removed == nil {
		t.Fatalf("nothing was uninstalled yet: %+v", got.Removed)
	}
	if len(r.plans) != 0 {
		t.Fatal("listing must not start anything")
	}
}

func TestUninstallAppsOnlyAcceptsInstalledCatalogApps(t *testing.T) {
	r := uninstApp(t)
	r.setInstalled("git", "vlc")
	wantErr(t, r.a.UninstallApps(nil), "nothing to remove")
	wantErr(t, r.a.UninstallApps([]string{"ghost"}), "not an app in the catalog")
	wantErr(t, r.a.UninstallApps([]string{"git", "../../etc"}), "not an app in the catalog")
	wantErr(t, r.a.UninstallApps([]string{"7zip"}), "not installed")
	wantErr(t, r.a.UninstallApps([]string{"git", "7zip"}), "not installed")
	if len(r.plans) != 0 {
		t.Fatalf("a refused request starts nothing: %+v", r.plans)
	}
	if err := r.a.UninstallApps([]string{"vlc", "git", "vlc"}); err != nil {
		t.Fatal(err)
	}
	if len(r.plans) != 1 || len(r.plans[0].Steps) != 2 {
		t.Fatalf("%+v", r.plans)
	}
	for i, id := range []string{"vlc", "git"} {
		s := r.plans[0].Steps[i]
		if s.Kind != install.StepUninstall || s.ID != id {
			t.Errorf("step %d: %+v", i, s)
		}
	}
	// The command the step runs is winget's silent, exact-id uninstall.
	args := strings.Join(install.WingetUninstallArgs(r.a.cat.Apps["git"]), " ")
	if !strings.Contains(args, "uninstall --id Git.Git --exact") || !strings.Contains(args, "--silent") {
		t.Fatalf("%s", args)
	}
}

func TestUninstallAppsRefusesWhileAnotherRunIsGoing(t *testing.T) {
	r := uninstApp(t)
	r.a.uninst.start = nil // the real start, which refuses before running anything
	r.setInstalled("git")
	r.a.running = true
	wantErr(t, r.a.UninstallApps([]string{"git"}), "already running")
	// A refused start must not make the app count as uninstalled this session.
	r.a.running = false
	r.setInstalled()
	v, err := r.a.UninstallVerify([]string{"git"})
	if err != nil || len(v.Removed) != 0 || len(v.Still) != 0 {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestUninstallVerifyConfirmsByScanningAgain(t *testing.T) {
	r := uninstApp(t)
	r.setInstalled("git", "vlc", "7zip")
	if err := r.a.UninstallApps([]string{"git", "vlc"}); err != nil {
		t.Fatal(err)
	}
	// winget said ok for both, but git is still detected: that is reported, not trusted.
	r.setInstalled("git", "7zip")
	v, err := r.a.UninstallVerify([]string{"git", "vlc", "7zip", "ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Removed) != 1 || v.Removed[0].ID != "vlc" || len(v.Still) != 1 || v.Still[0].ID != "git" {
		t.Fatalf("7zip was never uninstalled here and ghost is no app: %+v", v)
	}
	l, _ := r.a.UninstallList()
	if len(l.Removed) != 1 || l.Removed[0].ID != "vlc" || l.Removed[0].Name != "VLC" {
		t.Fatalf("%+v", l.Removed)
	}
	// Installed again outside WinForge: no longer "just removed".
	r.setInstalled("git", "7zip", "vlc")
	if l, _ = r.a.UninstallList(); len(l.Removed) != 0 {
		t.Fatalf("%+v", l.Removed)
	}
}

func TestLeftoversFindOnlyForAppsThatAreNotInstalled(t *testing.T) {
	r := uninstApp(t)
	r.setInstalled("git")
	_, err := r.a.LeftoversFind("git")
	wantErr(t, err, "still installed")
	_, err = r.a.LeftoversFind("ghost")
	wantErr(t, err, "not an app in the catalog")
	_, err = r.a.LeftoversFind("")
	wantErr(t, err, "not an app in the catalog")
	r.setInstalled()
	if _, err := r.a.LeftoversFind("git"); err != nil {
		t.Fatalf("a catalog app that is not installed can be searched: %v", err)
	}
}

func TestLeftoversFindListsOnlyMatchesAndNeverOtherInstalledApps(t *testing.T) {
	r := uninstApp(t)
	e := r.env
	write(t, filepath.Join(e.ProgramFiles, "Git", "bin", "git.exe"), 1000, 1)
	write(t, filepath.Join(e.ProgramFiles, "Git LFS", "x"), 10, 1)
	write(t, filepath.Join(e.AppData, "GitHub Desktop", "x"), 10, 1)
	write(t, filepath.Join(e.Home, "Documents", "Git", "mine.txt"), 10, 1)
	write(t, filepath.Join(e.Home, "Desktop", "Git.lnk"), 7, 1)
	write(t, filepath.Join(e.Home, "Desktop", "Git notes.txt"), 7, 1)
	r.setInstalled("gitlfs")
	got, err := r.a.LeftoversFind("git")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, c := range got.Candidates {
		paths = append(paths, c.Path)
	}
	sort.Strings(paths)
	want := []string{filepath.Join(e.Home, "Desktop", "Git.lnk"), filepath.Join(e.ProgramFiles, "Git")}
	sort.Strings(want)
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("%v", paths)
	}
	if got.AppID != "git" || got.AppName != "Git" || len(got.Searched) == 0 {
		t.Fatalf("%+v", got)
	}
	for _, c := range got.Candidates {
		if c.Why.Rule != uninstall.RuleName || c.Why.Value != "Git" {
			t.Errorf("%s: %+v", c.Path, c.Why)
		}
	}
	if r.bin.calls != 0 {
		t.Fatal("a search never recycles")
	}
	// Foo and Foo Pro share a package: with Foo Pro installed, Foo's folder is not Foo's alone.
	write(t, filepath.Join(e.AppData, "Foo", "x"), 10, 1)
	r.setInstalled("foopro")
	got, _ = r.a.LeftoversFind("foo")
	if len(got.Candidates) != 0 {
		t.Fatalf("%+v", got.Candidates)
	}
	r.setInstalled()
	if got, _ = r.a.LeftoversFind("foo"); len(got.Candidates) != 1 {
		t.Fatalf("%+v", got.Candidates)
	}
}

func TestLeftoversRecycleOnlyAcceptsWhatTheLastSearchListed(t *testing.T) {
	r := uninstApp(t)
	e := r.env
	write(t, filepath.Join(e.ProgramFiles, "Git", "a"), 100, 1)
	write(t, filepath.Join(e.AppData, "Git", "b"), 50, 1)
	write(t, filepath.Join(e.Home, "Documents", "keep.txt"), 5, 1)
	r.setInstalled()

	_, err := r.a.LeftoversRecycle(nil)
	wantErr(t, err, "nothing selected")
	// Nothing has been searched yet: no path is acceptable.
	_, err = r.a.LeftoversRecycle([]string{filepath.Join(e.AppData, "Git")})
	wantErr(t, err, "not listed")

	found, err := r.a.LeftoversFind("git")
	if err != nil || len(found.Candidates) != 2 {
		t.Fatalf("%+v %v", found, err)
	}
	for _, bad := range [][]string{
		{filepath.Join(e.Home, "Documents")},
		{filepath.Join(e.Home, "Documents", "keep.txt")},
		{e.ProgramFiles},
		{e.AppData},
		{filepath.Join(e.ProgramFiles, "Git") + string(filepath.Separator) + ".."},
		{filepath.Join(e.ProgramFiles, "Git", "a")},
		{filepath.Join(e.ProgramFiles, "Git"), filepath.Join(e.Home, "Documents")}, // one good, one not: refused whole
		{""},
	} {
		_, err := r.a.LeftoversRecycle(bad)
		wantErr(t, err, "not listed")
	}
	if r.bin.calls != 0 {
		t.Fatal("a refused request never reaches the Recycle Bin")
	}
	if !exists(filepath.Join(e.ProgramFiles, "Git", "a")) || !exists(filepath.Join(e.Home, "Documents", "keep.txt")) {
		t.Fatal("untouched")
	}
}

func TestLeftoversRecycleMovesReportsAndForgets(t *testing.T) {
	r := uninstApp(t)
	e := r.env
	pf, ad := filepath.Join(e.ProgramFiles, "Git"), filepath.Join(e.AppData, "Git")
	write(t, filepath.Join(pf, "a"), 100, 1)
	write(t, filepath.Join(ad, "b"), 50, 1)
	r.setInstalled()
	found, _ := r.a.LeftoversFind("git")
	if len(found.Candidates) != 2 {
		t.Fatalf("%+v", found)
	}
	// The Recycle Bin takes Program Files\Git, but AppData\Git is in use.
	r.bin.take = func() { _ = os.RemoveAll(pf) }
	res, err := r.a.LeftoversRecycle([]string{pf, ad, pf})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 1 || res.FreedBytes != 100 || len(res.Failed) != 1 || res.Failed[0].Path != ad || res.Failed[0].Reason != uninstall.ReasonNotMoved {
		t.Fatalf("%+v", res)
	}
	if !exists(ad) {
		t.Fatal("what could not be moved stays")
	}
	// What moved is no longer listed; what failed still is, so the person can try again.
	_, err = r.a.LeftoversRecycle([]string{pf})
	wantErr(t, err, "not listed")
	r.bin.take = func() { _ = os.RemoveAll(ad) }
	res, err = r.a.LeftoversRecycle([]string{ad})
	if err != nil || res.Moved != 1 || len(res.Failed) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestLeftoversRecycleRefusesWhenTheAppIsBackOrAnotherSearchReplacedTheList(t *testing.T) {
	r := uninstApp(t)
	e := r.env
	write(t, filepath.Join(e.AppData, "Git", "b"), 50, 1)
	write(t, filepath.Join(e.AppData, "VLC", "b"), 50, 1)
	r.setInstalled()
	if _, err := r.a.LeftoversFind("git"); err != nil {
		t.Fatal(err)
	}
	// Git is installed again before the person confirms.
	r.setInstalled("git")
	_, err := r.a.LeftoversRecycle([]string{filepath.Join(e.AppData, "Git")})
	wantErr(t, err, "still installed")
	if !exists(filepath.Join(e.AppData, "Git", "b")) || r.bin.calls != 0 {
		t.Fatal("untouched")
	}
	// A new search is the new truth: Git's folder is no longer on the list.
	r.setInstalled()
	if _, err := r.a.LeftoversFind("vlc"); err != nil {
		t.Fatal(err)
	}
	_, err = r.a.LeftoversRecycle([]string{filepath.Join(e.AppData, "Git")})
	wantErr(t, err, "not listed")
}

func TestLeftoversRecycleRefusesAFolderThatBecameALink(t *testing.T) {
	r := uninstApp(t)
	e := r.env
	outside := t.TempDir()
	write(t, filepath.Join(outside, "precious.txt"), 10, 1)
	target := filepath.Join(e.ProgramFiles, "Git")
	write(t, filepath.Join(target, "a"), 10, 1)
	r.setInstalled()
	if found, _ := r.a.LeftoversFind("git"); len(found.Candidates) != 1 {
		t.Fatalf("%+v", found)
	}
	// After the search, the folder is replaced by a link to somewhere else.
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		if out, err2 := mklinkJunction(target, outside); err2 != nil {
			t.Skipf("cannot create a link here: %v %s", err, out)
		}
	}
	r.bin.take = func() { _ = os.RemoveAll(outside) }
	res, err := r.a.LeftoversRecycle([]string{target})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 0 || len(res.Failed) != 1 || res.Failed[0].Reason != uninstall.ReasonLink || r.bin.calls != 0 || !exists(filepath.Join(outside, "precious.txt")) {
		t.Fatalf("%+v calls %d", res, r.bin.calls)
	}
}
