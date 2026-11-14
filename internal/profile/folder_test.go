package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/profile"
)

func save(t *testing.T, s *profile.Store, id, name string) {
	t.Helper()
	if err := s.Save(profile.FromInstalled(id, name, map[string]string{"git": ""}, false)); err != nil {
		t.Fatal(err)
	}
}

func writeAs(t *testing.T, dir, file, id, name string, mod time.Time) {
	t.Helper()
	b, err := profile.Export(profile.FromInstalled(id, name, map[string]string{"git": ""}, false))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDirAcceptsOnlyAFolderThatCanHoldProfiles(t *testing.T) {
	dir := t.TempDir()
	if err := profile.CheckDir(dir); err != nil {
		t.Fatalf("a real folder: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".winforge-write-test*")); len(left) != 0 {
		t.Fatalf("the write test left files behind: %v", left)
	}
	file := filepath.Join(dir, "a-file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{
		"empty":      "",
		"relative":   filepath.Join("some", "folder"),
		"missing":    filepath.Join(dir, "nope"),
		"a file":     file,
		"under file": filepath.Join(file, "child"),
	} {
		if err := profile.CheckDir(p); err == nil {
			t.Errorf("%s (%q) must be refused", name, p)
		}
	}
}

func TestTheSameFolderIsRecognisedWhateverItsSpelling(t *testing.T) {
	dir := t.TempDir()
	if !profile.SameDir(dir, filepath.Join(dir, "x", "..")) {
		t.Error("a path with .. is the same folder")
	}
	if !profile.SameDir(dir, dir+string(filepath.Separator)) {
		t.Error("a trailing separator does not change the folder")
	}
	if filepath.Separator == '\\' && !profile.SameDir(strings.ToUpper(dir), strings.ToLower(dir)) {
		t.Error("Windows paths ignore case")
	}
	if profile.SameDir(dir, filepath.Join(dir, "other")) {
		t.Error("different folders")
	}
}

func TestCopyToLeavesWhatIsAlreadyThere(t *testing.T) {
	src := &profile.Store{Dir: t.TempDir()}
	save(t, src, "alpha", "Alpha")
	save(t, src, "beta", "Beta")
	save(t, src, "gamma", "Gamma")
	// notes and a sync conflict copy are not profiles of ours and are not copied
	writeAs(t, src.Dir, "alpha-DESKTOP-OTHER.json", "alpha", "Alpha elsewhere", time.Now())
	if err := os.WriteFile(filepath.Join(src.Dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	writeAs(t, dst, "beta.json", "beta", "Beta from the other PC", time.Now())

	copied, skipped, err := src.CopyTo(dst)
	if err != nil || copied != 2 || skipped != 1 {
		t.Fatalf("copied %d skipped %d: %v", copied, skipped, err)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "beta.json"))
	if !strings.Contains(string(b), "Beta from the other PC") {
		t.Fatal("an existing file was overwritten")
	}
	for _, f := range []string{"alpha-DESKTOP-OTHER.json", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			t.Errorf("%s must not be copied", f)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dst, "*.tmp")); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
	// the originals stay where they were
	if list, _ := src.List(load(t)); len(list) != 3 {
		t.Fatalf("copying must not remove anything: %v", list)
	}
}

func TestCopyToAnUnusableFolderChangesNothing(t *testing.T) {
	src := &profile.Store{Dir: t.TempDir()}
	save(t, src, "alpha", "Alpha")
	if _, _, err := src.CopyTo(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("copying into a folder that is not there must fail")
	}
	if _, _, err := (&profile.Store{Dir: filepath.Join(t.TempDir(), "never-created")}).CopyTo(t.TempDir()); err != nil {
		t.Fatalf("no profiles yet is not an error: %v", err)
	}
}

func TestAConflictedCopyDoesNotShowTheProfileTwice(t *testing.T) {
	cat := load(t)
	dir := t.TempDir()
	old := time.Now().Add(-time.Hour)
	writeAs(t, dir, "alpha.json", "alpha", "Alpha (this PC)", old)
	writeAs(t, dir, "alpha-DESKTOP-OTHER.json", "alpha", "Alpha (other PC)", time.Now())
	writeAs(t, dir, "beta.json", "beta", "Beta", old)
	list, err := (&profile.Store{Dir: dir}).List(cat)
	if err != nil || len(list) != 2 {
		t.Fatalf("one entry per profile: %v %v", list, err)
	}
	for _, p := range list {
		if p.ID == "alpha" && p.Name != "Alpha (other PC)" {
			t.Fatalf("the most recently written copy wins: %q", p.Name)
		}
	}
}

func TestOnATieTheFileNamedAfterTheProfileWins(t *testing.T) {
	cat := load(t)
	dir := t.TempDir()
	at := time.Now().Add(-time.Minute)
	writeAs(t, dir, "alpha-copy.json", "alpha", "Copy", at)
	writeAs(t, dir, "alpha.json", "alpha", "Original", at)
	list, _ := (&profile.Store{Dir: dir}).List(cat)
	if len(list) != 1 || list[0].Name != "Original" {
		t.Fatalf("%v", list)
	}
}

func TestDeletingAProfileAlsoRemovesItsConflictedCopies(t *testing.T) {
	cat := load(t)
	dir := t.TempDir()
	s := &profile.Store{Dir: dir}
	now := time.Now()
	writeAs(t, dir, "alpha.json", "alpha", "Alpha", now)
	writeAs(t, dir, "alpha (conflicted copy).json", "alpha", "Alpha 2", now)
	writeAs(t, dir, "beta.json", "beta", "Beta", now)
	if err := os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{"not":"a profile"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("alpha"); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(cat)
	if len(list) != 1 || list[0].ID != "beta" {
		t.Fatalf("alpha must be gone for good: %v", list)
	}
	for _, f := range []string{"beta.json", "other.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s must be left alone: %v", f, err)
		}
	}
}
