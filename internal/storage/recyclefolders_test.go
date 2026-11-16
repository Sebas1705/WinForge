package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func TestRecycleFoldersCountsOnlyFoldersThatAreGoneAndMeasuresThem(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	write(t, filepath.Join(a, "x", "one.bin"), make([]byte, 100), now)
	write(t, filepath.Join(a, "two.bin"), make([]byte, 50), now)
	write(t, filepath.Join(b, "three.bin"), make([]byte, 10), now)
	run := &fakeRunner{effect: func(string, []string) { _ = os.RemoveAll(a) }} // b is in use
	moved, freed, failed := storage.RecycleFolders(context.Background(), run, []string{a, b})
	if moved != 1 || freed != 150 || len(failed) != 1 || failed[0] != b {
		t.Fatalf("moved %d freed %d failed %v", moved, freed, failed)
	}
	if !exists(b) {
		t.Fatal("a folder that could not be moved stays whole")
	}
}

func TestRecycleFoldersRefusesFilesMissingPathsAndLinks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.bin")
	write(t, file, []byte("x"), now)
	run := &fakeRunner{}
	m, _, failed := storage.RecycleFolders(context.Background(), run, []string{file, filepath.Join(dir, "missing")})
	if m != 0 || len(failed) != 2 || len(run.calls) != 0 {
		t.Fatalf("a file or a missing path is not a folder: %d %v calls %d", m, failed, len(run.calls))
	}
	// And the file recycler still refuses folders.
	if m, _, fl := storage.Recycle(context.Background(), run, []string{dir}); m != 0 || len(fl) != 1 || len(run.calls) != 0 {
		t.Fatalf("Recycle must stay files-only: %d %v", m, fl)
	}
	target := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a link here: %v", err)
	}
	if m, _, fl := storage.RecycleFolders(context.Background(), run, []string{link}); m != 0 || len(fl) != 1 || len(run.calls) != 0 {
		t.Fatalf("a link is never moved: %d %v", m, fl)
	}
	if m, _, fl := storage.RecycleFolders(context.Background(), nil, []string{dir}); m != 0 || len(fl) != 1 {
		t.Fatalf("no runner, nothing moves: %d %v", m, fl)
	}
}

func TestRecycleFoldersUsesTheDirectoryCallAndKeepsNamesOutOfTheCommand(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "a'; Remove-Item -Recurse #$(calc)")
	if os.PathSeparator == '\\' {
		odd = filepath.Join(dir, "a'; Remove-Item #$(calc)")
	}
	write(t, filepath.Join(odd, "f"), []byte("x"), now)
	run := &fakeRunner{}
	storage.RecycleFolders(context.Background(), run, []string{odd})
	if len(run.calls) != 1 {
		t.Fatalf("%v", run.calls)
	}
	script := decode(t, run.calls[0])
	if !strings.Contains(script, "DeleteDirectory") || strings.Contains(script, "DeleteFile") {
		t.Fatalf("folders go through DeleteDirectory: %s", script)
	}
	for _, leak := range []string{"Remove-Item", "calc"} {
		if strings.Contains(script, leak) {
			t.Fatalf("a folder name leaked into the command (%q): %s", leak, script)
		}
	}
	// The file recycler still uses DeleteFile.
	f := filepath.Join(dir, "plain.bin")
	write(t, f, []byte("x"), now)
	run = &fakeRunner{}
	storage.Recycle(context.Background(), run, []string{f})
	if s := decode(t, run.calls[0]); !strings.Contains(s, "DeleteFile") || strings.Contains(s, "DeleteDirectory") {
		t.Fatalf("%s", s)
	}
}
