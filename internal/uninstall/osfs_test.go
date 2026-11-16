package uninstall_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/uninstall"
)

// recycler stands in for PowerShell: it records the calls and removes the paths it is told to "recycle".
type recycler struct {
	mu    sync.Mutex
	calls int
	take  func() // what the Recycle Bin does when asked
}

func (r *recycler) Run(context.Context, string, ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.take != nil {
		r.take()
	}
	return nil, nil
}

func write(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

// realPC lays the fixed places out on the real disk, in a temporary folder.
func realPC(t *testing.T) uninstall.Env {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	e := uninstall.Env{
		Home: home, AppData: filepath.Join(home, "AppData", "Roaming"), LocalAppData: filepath.Join(home, "AppData", "Local"),
		ProgramData: filepath.Join(root, "ProgramData"), ProgramFiles: filepath.Join(root, "Program Files"),
		ProgramFilesX86: filepath.Join(root, "Program Files (x86)"), WinDir: filepath.Join(root, "Windows"), SystemDrive: root,
		Public: filepath.Join(root, "Users", "Public"),
	}
	for _, d := range []string{e.AppData, e.LocalAppData, e.ProgramData, e.ProgramFiles, e.ProgramFilesX86, e.WinDir,
		filepath.Join(home, "Desktop"), filepath.Join(home, "Documents"),
		filepath.Join(e.AppData, "Microsoft", "Windows", "Start Menu", "Programs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// sys is the real disk with a runner of the test's choosing and no programs installed.
func sys(rec *recycler) uninstall.System {
	return uninstall.NewSystemWithPrograms(rec, func() []uninstall.RegApp { return nil })
}

// dirLink makes link a link to the folder target: a symbolic link, or on Windows (where that needs a
// privilege) a junction, which is what most links there are.
func dirLink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Symlink(target, link); err == nil {
		return
	} else if runtime.GOOS != "windows" {
		t.Skipf("cannot create a link here: %v", err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("cannot create a junction here: %v %s", err, out)
	}
}

func TestRealDiskFindAndRecycle(t *testing.T) {
	e := realPC(t)
	write(t, filepath.Join(e.ProgramFiles, "Git", "bin", "git.exe"), 1000)
	write(t, filepath.Join(e.ProgramFiles, "Git", "README"), 24)
	write(t, filepath.Join(e.AppData, "Git", "cfg"), 5)
	write(t, filepath.Join(e.Home, "Desktop", "Git.lnk"), 9)
	write(t, filepath.Join(e.Home, "Desktop", "Git.txt"), 9)
	write(t, filepath.Join(e.Home, "Documents", "Git", "mine.txt"), 9)
	write(t, filepath.Join(e.ProgramFiles, "GitHub", "x"), 9)

	rec := &recycler{}
	f := uninstall.Finder{Sys: sys(rec), Env: e}
	got, searched, err := f.Find(context.Background(), git, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(searched) == 0 {
		t.Fatal("it must say where it looked")
	}
	if len(got) != 3 {
		t.Fatalf("%v", paths(got))
	}
	for _, c := range got {
		if c.Path == filepath.Join(e.ProgramFiles, "Git") && (c.SizeBytes != 1024 || c.Files != 2) {
			t.Fatalf("measured %d bytes in %d files", c.SizeBytes, c.Files)
		}
		if c.Modified.IsZero() {
			t.Fatalf("%s has no modified time", c.Path)
		}
	}
	if rec.calls != 0 {
		t.Fatal("a search never recycles")
	}

	// The Recycle Bin takes the program folder but is "busy" with the others.
	rec.take = func() { _ = os.RemoveAll(filepath.Join(e.ProgramFiles, "Git")) }
	res := f.Remove(context.Background(), git, nil, got)
	if res.Moved != 1 || res.FreedBytes != 1024 || len(res.Failed) != 2 {
		t.Fatalf("%+v", res)
	}
	for _, p := range []string{filepath.Join(e.AppData, "Git"), filepath.Join(e.Home, "Desktop", "Git.lnk"), filepath.Join(e.Home, "Desktop", "Git.txt"),
		filepath.Join(e.Home, "Documents", "Git", "mine.txt"), filepath.Join(e.ProgramFiles, "GitHub", "x")} {
		if !exists(p) {
			t.Errorf("%s must still be there", p)
		}
	}
}

func TestRealDiskNeverFollowsOrMovesLinks(t *testing.T) {
	e := realPC(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "precious.txt"), 10)
	dirLink(t, filepath.Join(e.ProgramFiles, "Git"), outside)
	fileLinked := os.Symlink(filepath.Join(outside, "precious.txt"), filepath.Join(e.Home, "Desktop", "Git.lnk")) == nil
	rec := &recycler{take: func() { _ = os.RemoveAll(outside) }}
	f := uninstall.Finder{Sys: sys(rec), Env: e}
	got, _, err := f.Find(context.Background(), git, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("links are not listed: %v %v", paths(got), err)
	}
	// Even when asked for by path, they are refused before the Recycle Bin is called.
	items := []uninstall.Candidate{{Path: filepath.Join(e.ProgramFiles, "Git"), Kind: uninstall.KindFolder}}
	if fileLinked {
		items = append(items, uninstall.Candidate{Path: filepath.Join(e.Home, "Desktop", "Git.lnk"), Kind: uninstall.KindShortcut})
	}
	res := f.Remove(context.Background(), git, nil, items)
	if res.Moved != 0 || rec.calls != 0 || !exists(filepath.Join(outside, "precious.txt")) || len(res.Failed) != len(items) {
		t.Fatalf("%+v calls %d", res, rec.calls)
	}
	for _, f := range res.Failed {
		if f.Reason != uninstall.ReasonLink {
			t.Errorf("%s: %s", f.Path, f.Reason)
		}
	}
}

func TestRealDiskWithNoRunnerMovesNothing(t *testing.T) {
	e := realPC(t)
	write(t, filepath.Join(e.AppData, "Git", "cfg"), 5)
	f := uninstall.Finder{Sys: uninstall.NewSystemWithPrograms(nil, func() []uninstall.RegApp { return nil }), Env: e}
	got, _, _ := f.Find(context.Background(), git, nil)
	res := f.Remove(context.Background(), git, nil, got)
	if res.Moved != 0 || len(res.Failed) != 1 || !exists(filepath.Join(e.AppData, "Git", "cfg")) {
		t.Fatalf("%+v", res)
	}
}

func TestRealDiskReadsAnEmptyOrMissingPlaceQuietly(t *testing.T) {
	e := realPC(t)
	e.ProgramFilesX86 = filepath.Join(t.TempDir(), "does-not-exist")
	f := uninstall.Finder{Sys: sys(&recycler{}), Env: e}
	got, searched, err := f.Find(context.Background(), git, nil)
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	for _, s := range searched {
		if s.Place == uninstall.PlaceProgramFilesX86 {
			t.Fatal("a missing place is not searched")
		}
	}
}
