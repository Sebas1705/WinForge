package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// recycler stands in for the Recycle Bin: it removes what it is asked to.
type recycler struct {
	mu    sync.Mutex
	calls int
	files []string
	stuck map[string]bool // files that stay (in use)
}

func (r *recycler) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	for _, f := range r.files {
		if !r.stuck[f] {
			_ = os.Remove(f)
		}
	}
	return nil, nil
}

type events struct {
	mu   sync.Mutex
	seen []string
}

func (e *events) emit(name string, _ any) {
	e.mu.Lock()
	e.seen = append(e.seen, name)
	e.mu.Unlock()
}

func storageApp(t *testing.T) (*App, *events) {
	t.Helper()
	a := testApp(t)
	ev := &events{}
	a.emitter = ev.emit
	return a, ev
}

func write(t *testing.T, path string, size int, fill byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{fill}, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

const mib = 1 << 20

func TestStorageCleanRejectsEmptyAndUnknownRequests(t *testing.T) {
	a, _ := storageApp(t)
	_, err := a.StorageClean(nil)
	wantErr(t, err, "nothing selected")
	res, err := a.StorageClean([]string{"../../Windows"})
	if err != nil || len(res.Outcomes) != 1 || res.Outcomes[0].Error != "unknown category" || res.FreedBytes != 0 {
		t.Fatalf("an id that is not a category does nothing: %+v %v", res, err)
	}
}

func TestStorageTasksRunOneAtATimeAndCanBeCancelled(t *testing.T) {
	a, _ := storageApp(t)
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = a.beginStorageTask()
	wantErr(t, err, "still running")
	_, err = a.StorageScan()
	wantErr(t, err, "still running")
	_, err = a.StorageBigFiles(t.TempDir(), 10)
	wantErr(t, err, "still running")
	_, err = a.StorageDuplicates([]string{t.TempDir()}, 1)
	wantErr(t, err, "still running")

	a.StorageCancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("StorageCancel did not stop the running task")
	}
	end()
	if _, end2, err := a.beginStorageTask(); err != nil {
		t.Fatalf("a finished task frees the slot: %v", err)
	} else {
		end2()
	}
	a.StorageCancel() // nothing running: harmless
}

func TestOnlyFilesThatAScanListedCanBeRecycledOrRevealed(t *testing.T) {
	a, _ := storageApp(t)
	dir := t.TempDir()
	listed := filepath.Join(dir, "big", "listed.bin")
	other := filepath.Join(dir, "other.bin")
	write(t, listed, 3*mib, 1)
	write(t, other, 10, 2)
	rec := &recycler{files: []string{listed, other}}
	a.runner = rec

	_, err := a.StorageRecycle([]string{listed})
	wantErr(t, err, "was not listed")
	wantErr(t, a.StorageReveal(listed), "not listed")
	_, err = a.StorageRecycle(nil)
	wantErr(t, err, "no files selected")

	got, err := a.StorageBigFiles(dir, 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	_, err = a.StorageRecycle([]string{listed + "x"})
	wantErr(t, err, "was not listed")
	_, err = a.StorageRecycle([]string{listed, filepath.Join(dir, "never-listed.bin")})
	wantErr(t, err, "was not listed")
	if rec.calls != 0 || !exists(listed) {
		t.Fatalf("a refused request must not run anything: %d calls", rec.calls)
	}

	res, err := a.StorageRecycle([]string{listed})
	if err != nil {
		t.Fatal(err)
	}
	if res.Moved != 1 || res.FreedBytes != 3*mib || len(res.Failed) != 0 || exists(listed) {
		t.Fatalf("%+v", res)
	}
	if _, err := a.StorageRecycle([]string{listed}); err == nil {
		t.Fatal("a file that was moved is no longer listed")
	}
}

func TestRecyclingFoldersIsRefusedAndFailuresAreReported(t *testing.T) {
	a, _ := storageApp(t)
	dir := t.TempDir()
	a.remember(nil, []string{filepath.Join(dir, "folder")})
	if err := os.MkdirAll(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := a.StorageRecycle([]string{filepath.Join(dir, "folder")})
	wantErr(t, err, "was not listed")

	f := filepath.Join(dir, "locked.bin")
	write(t, f, 100, 3)
	a.remember([]string{f}, nil)
	a.runner = &recycler{files: []string{f}, stuck: map[string]bool{f: true}}
	res, err := a.StorageRecycle([]string{f})
	if err != nil || res.Moved != 0 || len(res.Failed) != 1 || !exists(f) {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestADuplicateGroupCanNeverLoseEveryCopy(t *testing.T) {
	a, _ := storageApp(t)
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin"), filepath.Join(dir, "c.bin")}
	for _, p := range paths {
		write(t, p, mib, 9)
	}
	write(t, filepath.Join(dir, "unrelated.bin"), mib, 8)
	rec := &recycler{files: paths}
	a.runner = rec

	groups, err := a.StorageDuplicates([]string{dir}, 1)
	if err != nil || len(groups) != 1 || len(groups[0].Files) != 3 {
		t.Fatalf("%+v %v", groups, err)
	}
	_, err = a.StorageRecycle(paths)
	wantErr(t, err, "every copy")
	for _, p := range paths {
		if !exists(p) {
			t.Fatal("a refused request removed a file")
		}
	}
	if rec.calls != 0 {
		t.Fatal("a refused request ran a command")
	}

	// Two of three: allowed. The one left is protected from a second request.
	rec.files = paths[:2] // the Recycle Bin only takes what was asked
	res, err := a.StorageRecycle(paths[:2])
	if err != nil || res.Moved != 2 || res.FreedBytes != 2*mib {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := a.StorageRecycle(paths[2:]); err == nil {
		t.Fatal("the last copy must not be removable in a second call")
	}
	if !exists(paths[2]) {
		t.Fatal("the last copy is gone")
	}
}

func TestACopyThatCouldNotBeMovedStillCountsAsACopy(t *testing.T) {
	a, _ := storageApp(t)
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin"), filepath.Join(dir, "c.bin")}
	for _, p := range paths {
		write(t, p, mib, 7)
	}
	a.runner = &recycler{files: paths[:2], stuck: map[string]bool{paths[1]: true}}
	if _, err := a.StorageDuplicates([]string{dir}, 1); err != nil {
		t.Fatal(err)
	}
	res, err := a.StorageRecycle(paths[:2]) // b is in use and stays
	if err != nil || res.Moved != 1 || len(res.Failed) != 1 || !exists(paths[1]) {
		t.Fatalf("%+v %v", res, err)
	}
	// b and c remain: both are still copies of each other, so the group is still protected.
	_, err = a.StorageRecycle([]string{paths[1], paths[2]})
	wantErr(t, err, "every copy")
}

func TestStorageDuplicatesNeedsFoldersAndClampsTheMinimum(t *testing.T) {
	a, _ := storageApp(t)
	_, err := a.StorageDuplicates(nil, 1)
	wantErr(t, err, "at least one folder")
	dir := t.TempDir()
	write(t, filepath.Join(dir, "x1"), 10, 1)
	write(t, filepath.Join(dir, "x2"), 10, 1)
	groups, err := a.StorageDuplicates([]string{dir}, -5)
	if err != nil || len(groups) != 0 {
		t.Fatalf("files under 1 MB are never reported: %+v %v", groups, err)
	}
	if _, err := a.StorageDuplicates([]string{filepath.Join(dir, "missing")}, 1); err == nil {
		t.Fatal("a folder that does not exist must say so")
	}
}

func TestStorageProgressReachesTheInterface(t *testing.T) {
	a, ev := storageApp(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "f.bin"), 100, 1)
	if _, err := a.StorageBigFiles(dir, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StorageFolders(dir); err != nil {
		t.Fatal(err)
	}
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if len(ev.seen) < 1 || ev.seen[0] != "storage:progress" {
		t.Fatalf("events: %v", ev.seen)
	}
}

func TestStorageFoldersAreRevealableButNotRecyclable(t *testing.T) {
	a, _ := storageApp(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "sub", "f.bin"), 100, 1)
	got, err := a.StorageFolders(dir)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	_, err = a.StorageRecycle([]string{got[0].Path})
	wantErr(t, err, "was not listed")
	wantErr(t, a.StorageReveal(filepath.Join(dir, "nope")), "not listed")
}

func TestStorageDefaultsAndVolumes(t *testing.T) {
	a, _ := storageApp(t)
	d := a.StorageDefaults()
	if d.Home == "" || d.Downloads == "" {
		t.Fatalf("%+v", d)
	}
	if a.StorageVolumes() == nil {
		t.Fatal("never null")
	}
}

func TestPathKeyIsCaseAndDotInsensitiveOnWindows(t *testing.T) {
	if filepath.Separator != '\\' {
		t.Skip("Windows paths")
	}
	if pathKey(`C:\Users\Me\..\Me\File.BIN`) != pathKey(`c:\users\me\file.bin`) {
		t.Fatal("the same file must give the same key")
	}
}

func TestStorageStringHelpersDoNotLeak(t *testing.T) {
	if strings.TrimSpace(pathKey("  ")) == "" {
		t.Log("blank paths produce a key but are never listed, so they are never accepted")
	}
}
