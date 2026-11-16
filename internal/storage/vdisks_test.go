package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func TestVirtualDisksAreListedBiggestFirstWithReadableNames(t *testing.T) {
	e := fakeEnv(t)
	put(t, filepath.Join(e.LocalAppData, "Docker", "wsl", "disk", "docker_data.vhdx"), 5000, 0)
	put(t, filepath.Join(e.LocalAppData, "Docker", "wsl", "main", "ext4.vhdx"), 300, 0)
	put(t, filepath.Join(e.LocalAppData, "Packages", "CanonicalGroupLimited.Ubuntu22.04LTS_79rhkp1fndgsc", "LocalState", "ext4.vhdx"), 9000, 0)
	put(t, filepath.Join(e.LocalAppData, "Packages", "TheDebianProject.DebianGNULinux_76v4gfsz19hv4", "LocalState", "ext4.vhdx"), 700, 0)
	put(t, filepath.Join(e.LocalAppData, "wsl", "{1234-abcd}", "ext4.vhdx"), 1200, 0)
	// not disk images, or not ours
	put(t, filepath.Join(e.LocalAppData, "Packages", "Some.App_x", "LocalState", "settings.json"), 50, 0)
	put(t, filepath.Join(e.LocalAppData, "Packages", "Some.App_x", "LocalState", "deep", "ext4.vhdx"), 50, 0)
	put(t, filepath.Join(e.LocalAppData, "elsewhere", "ext4.vhdx"), 50, 0)

	got := storage.VirtualDisks(e)
	if len(got) != 5 {
		t.Fatalf("%d disks: %+v", len(got), got)
	}
	wantSizes := []int64{9000, 5000, 1200, 700, 300}
	for i, d := range got {
		if d.SizeBytes != wantSizes[i] {
			t.Errorf("%d: %+v", i, d)
		}
	}
	if got[0].Name != "Ubuntu22.04LTS" || got[0].Kind != "wsl" {
		t.Errorf("%+v", got[0])
	}
	if got[1].Name != "Docker Desktop" || got[1].Kind != "docker" {
		t.Errorf("%+v", got[1])
	}
	if got[3].Name != "DebianGNULinux" {
		t.Errorf("%+v", got[3])
	}
	if got[2].Name != "{1234-abcd}" {
		t.Errorf("a distribution kept in the newer place is named by its folder: %+v", got[2])
	}
}

func TestNoVirtualDisksIsAnEmptyListNotNull(t *testing.T) {
	if got := storage.VirtualDisks(fakeEnv(t)); got == nil || len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if got := storage.VirtualDisks(storage.Env{}); got == nil || len(got) != 0 {
		t.Fatalf("with no folders configured: %v", got)
	}
}
