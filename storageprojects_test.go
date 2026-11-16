package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// projectApp is an App whose storage works inside a temporary home folder, with one project that has been
// left alone for 200 days and one that was touched a minute ago. Nothing on this PC is read or changed.
func projectApp(t *testing.T) (*App, *events, string) {
	t.Helper()
	a, ev := storageApp(t)
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	env := storage.Env{
		Home: home, LocalAppData: filepath.Join(home, "AppData", "Local"), AppData: filepath.Join(home, "AppData", "Roaming"),
		ProgramData: filepath.Join(root, "ProgramData"), WinDir: filepath.Join(root, "Windows"), SystemDrive: root,
		Temp: filepath.Join(home, "AppData", "Local", "Temp"),
	}
	a.env = &env
	for _, p := range []struct {
		name string
		age  time.Duration
	}{{"old", 200 * 24 * time.Hour}, {"fresh", time.Minute}} {
		proj := filepath.Join(home, "code", p.name)
		write(t, filepath.Join(proj, "package.json"), 20, 1)
		write(t, filepath.Join(proj, "node_modules", "dep", "index.js"), 3*mib, 2)
		mt := time.Now().Add(-p.age)
		for _, f := range []string{filepath.Join(proj, "package.json"), proj} {
			if err := os.Chtimes(f, mt, mt); err != nil {
				t.Fatal(err)
			}
		}
	}
	return a, ev, home
}

func TestStorageProjectsListsOnlyProjectsLeftAlone(t *testing.T) {
	a, ev, home := projectApp(t)
	_, err := a.StorageProjects(nil, 90)
	wantErr(t, err, "choose at least one folder")

	got, err := a.StorageProjects([]string{filepath.Join(home, "code")}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project != "old" || got[0].Kind != "node_modules" || got[0].SizeBytes != 3*mib {
		t.Fatalf("%+v", got)
	}
	// the age is clamped: asking for 0 days still never lists a project touched a minute ago
	got, err = a.StorageProjects([]string{filepath.Join(home, "code")}, 0)
	if err != nil || len(got) != 1 || got[0].Project != "old" {
		t.Fatalf("%+v %v", got, err)
	}
	_ = ev
}

func TestStorageProjectsCleanOnlyAcceptsWhatTheSearchListed(t *testing.T) {
	a, _, home := projectApp(t)
	oldJunk := filepath.Join(home, "code", "old", "node_modules")
	freshJunk := filepath.Join(home, "code", "fresh", "node_modules")

	_, err := a.StorageProjectsClean(nil)
	wantErr(t, err, "nothing selected")
	_, err = a.StorageProjectsClean([]string{oldJunk})
	wantErr(t, err, "was not listed by a project search")
	if !exists(oldJunk) {
		t.Fatal("a refused request must not delete anything")
	}

	if _, err := a.StorageProjects([]string{filepath.Join(home, "code")}, 90); err != nil {
		t.Fatal(err)
	}
	// a folder the search did not list is refused even beside one it did, and nothing at all is removed
	_, err = a.StorageProjectsClean([]string{oldJunk, freshJunk})
	wantErr(t, err, "was not listed by a project search")
	if !exists(oldJunk) || !exists(freshJunk) {
		t.Fatal("one unlisted path must stop the whole request")
	}
	_, err = a.StorageProjectsClean([]string{oldJunk + "x"})
	wantErr(t, err, "was not listed by a project search")

	res, err := a.StorageProjectsClean([]string{oldJunk})
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 || res.FreedBytes != 3*mib || len(res.Failed) != 0 || exists(oldJunk) {
		t.Fatalf("%+v", res)
	}
	if !exists(filepath.Join(home, "code", "old", "package.json")) || !exists(freshJunk) {
		t.Fatal("only the listed build folder goes: the project's own files and other projects stay")
	}
	if _, err := a.StorageProjectsClean([]string{oldJunk}); err == nil {
		t.Fatal("a folder that was removed is no longer listed")
	}
}

func TestAProjectTouchedAfterTheSearchIsRefusedAndStaysListed(t *testing.T) {
	a, _, home := projectApp(t)
	junk := filepath.Join(home, "code", "old", "node_modules")
	if _, err := a.StorageProjects([]string{filepath.Join(home, "code")}, 90); err != nil {
		t.Fatal(err)
	}
	// the person starts working on it between the search and the click
	now := time.Now()
	if err := os.Chtimes(filepath.Join(home, "code", "old", "package.json"), now, now); err != nil {
		t.Fatal(err)
	}
	res, err := a.StorageProjectsClean([]string{junk})
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 0 || len(res.Failed) != 1 || len(res.Reasons) != 1 || !exists(junk) {
		t.Fatalf("a project that is in use again must be left alone: %+v", res)
	}
	// it stays listed, so it can be tried again after the project is idle
	a.storage.mu.Lock()
	listed := a.storage.projects[pathKey(junk)]
	a.storage.mu.Unlock()
	if !listed {
		t.Fatal("a refused folder stays in the list")
	}
}

func TestProjectTasksShareTheSingleStorageJob(t *testing.T) {
	a, _, home := projectApp(t)
	a.storage.mu.Lock()
	a.storage.busy = true
	a.storage.mu.Unlock()
	_, err := a.StorageProjects([]string{filepath.Join(home, "code")}, 90)
	wantErr(t, err, "another")
}

func TestVirtualDisksAreRevealOnlyNeverRecyclable(t *testing.T) {
	a, _, home := projectApp(t)
	disk := filepath.Join(home, "AppData", "Local", "Packages", "Canonical.Ubuntu_x", "LocalState", "ext4.vhdx")
	write(t, disk, 2*mib, 3)

	got := a.StorageVirtualDisks()
	if len(got) != 1 || got[0].Path != disk || got[0].Kind != "wsl" || got[0].SizeBytes != 2*mib {
		t.Fatalf("%+v", got)
	}
	a.storage.mu.Lock()
	reveal, recycle := a.storage.folders[pathKey(disk)], a.storage.listed[pathKey(disk)]
	a.storage.mu.Unlock()
	if !reveal {
		t.Fatal("a listed disk can be shown in Explorer")
	}
	if recycle {
		t.Fatal("a virtual disk must never be offered for removal")
	}
	rec := &recycler{files: []string{disk}}
	a.runner = rec
	_, err := a.StorageRecycle([]string{disk})
	wantErr(t, err, "was not listed")
	if rec.calls != 0 || !exists(disk) {
		t.Fatal("nothing runs for a virtual disk")
	}

}
