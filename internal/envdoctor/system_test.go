package envdoctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These touch only a temp folder, never the registry or the real environment.

func TestOSStatKinds(t *testing.T) {
	sys := NewSystem()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if sys.Stat(dir) != Dir || sys.Stat(file) != File || sys.Stat(filepath.Join(dir, "nope")) != Missing {
		t.Error("dir, file and missing")
	}
}

func TestOSStatFollowsSymlinks(t *testing.T) {
	sys := NewSystem()
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if sys.Stat(link) != Dir {
		t.Error("a link to a folder is a folder")
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Skip(err)
	}
	if sys.Stat(broken) == Dir {
		t.Error("a broken link is not a folder")
	}
}

func TestBackupDirIsInsideWinForgesFolder(t *testing.T) {
	d, err := NewSystem().BackupDir()
	if err != nil {
		t.Skip(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(d), "WinForge/env-backups") {
		t.Errorf("%q", d)
	}
}

func TestOSStatNeverRunsAnything(t *testing.T) {
	// A file with an executable name is only looked at.
	dir := t.TempDir()
	exe := filepath.Join(dir, "java.exe")
	if err := os.WriteFile(exe, []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if NewSystem().Stat(exe) != File {
		t.Error("found by existence alone")
	}
}
