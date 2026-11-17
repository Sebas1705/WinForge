//go:build windows

package offlinekit

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// A junction is a folder that points elsewhere without being a symbolic link; it is made in a temporary
// folder with the same tool a person (or an attacker with a USB stick) would use.
func TestOSFSSeesAJunctionAsALink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	fs := OSFS{}
	if err := fs.MkdirAll(target); err != nil {
		t.Fatal(err)
	}
	junction := filepath.Join(dir, "junction")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Skipf("cannot make a junction here: %v %s", err, out)
	}
	info, err := fs.Lstat(junction)
	if err != nil || !info.Link || info.Dir {
		t.Fatalf("junction is %+v (%v), want a link", info, err)
	}
	ents, err := fs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.Name == "junction" && (!e.Link || e.Dir) {
			t.Fatalf("listed as %+v", e)
		}
	}
	free, err := fs.FreeBytes(dir)
	if err != nil || free <= 0 {
		t.Fatalf("free space %d %v", free, err)
	}
	// The kit folder may not exist yet: the drive that would hold it is measured.
	if free, err := fs.FreeBytes(filepath.Join(dir, "not", "yet")); err != nil || free <= 0 {
		t.Fatalf("free space of a missing folder %d %v", free, err)
	}
}
