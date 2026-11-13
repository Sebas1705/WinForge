package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func TestParallelScanKeepsTheOrderAndTheExactTotals(t *testing.T) {
	e := fakeEnv(t)
	day := 24 * time.Hour
	want := map[string]int64{}
	add := func(id, path string, size int) {
		put(t, path, size, 3*day)
		want[id] += int64(size)
	}
	for i := 0; i < 40; i++ {
		add("temp-user", filepath.Join(e.Temp, fmt.Sprintf("d%d", i%5), fmt.Sprintf("t%d", i)), 100+i)
		add("dev-npm", filepath.Join(e.LocalAppData, "npm-cache", fmt.Sprintf("n%d", i)), 50+i)
		add("dev-go", filepath.Join(e.LocalAppData, "go-build", fmt.Sprintf("g%d", i)), 70+i)
		add("windows-update", filepath.Join(e.WinDir, "SoftwareDistribution", "Download", fmt.Sprintf("u%d", i)), 10+i)
		add("shader-cache", filepath.Join(e.LocalAppData, "D3DSCache", fmt.Sprintf("s%d", i)), 5+i)
	}
	var reports atomic.Int32
	o := opts(e)
	o.Progress = func(storage.Progress) { reports.Add(1) }
	for round := 0; round < 5; round++ {
		cs, err := storage.Scan(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		if len(cs) != len(storage.Specs) {
			t.Fatalf("%d categories", len(cs))
		}
		for i, c := range cs {
			if c.ID != storage.Specs[i].ID {
				t.Fatalf("round %d: position %d is %s, want %s (parallel work must not reorder results)", round, i, c.ID, storage.Specs[i].ID)
			}
			if w := want[c.ID]; c.Bytes != w {
				t.Fatalf("round %d: %s counted %d bytes, want %d", round, c.ID, c.Bytes, w)
			}
		}
	}
	if reports.Load() < 5 {
		t.Fatalf("each scan ends with a final progress report, got %d", reports.Load())
	}
}

func TestParallelFolderSizesAreTheSameAsCountingByHand(t *testing.T) {
	root := t.TempDir()
	want := map[string]int64{}
	for f := 0; f < 25; f++ {
		name := fmt.Sprintf("folder%02d", f)
		for i := 0; i <= f; i++ {
			write(t, filepath.Join(root, name, fmt.Sprintf("sub%d", i%3), fmt.Sprintf("f%d", i)), bytes.Repeat([]byte{1}, 10+f), now)
			want[name] += int64(10 + f)
		}
	}
	got, err := storage.FolderSizes(context.Background(), root, nil)
	if err != nil || len(got) != 25 {
		t.Fatalf("%v %d", err, len(got))
	}
	for i, f := range got {
		if f.SizeBytes != want[f.Name] || f.Files == 0 {
			t.Errorf("%s: %d bytes, want %d", f.Name, f.SizeBytes, want[f.Name])
		}
		if i > 0 && got[i-1].SizeBytes < f.SizeBytes {
			t.Errorf("not sorted at %d", i)
		}
	}
	if got[0].Name != "folder24" {
		t.Fatalf("biggest first: %s", got[0].Name)
	}
}

func TestDuplicatesIgnoreBuildOutputOfProjects(t *testing.T) {
	root := t.TempDir()
	lib := bytes.Repeat([]byte("L"), 3000)
	for _, dir := range []string{"app/build/intermediates/lib", "other/build", "web/dist", "rust/target/debug", "py/.venv/lib", "py/__pycache__", ".gradle/caches"} {
		write(t, filepath.Join(root, filepath.FromSlash(dir), "libx.so"), lib, now)
	}
	real := bytes.Repeat([]byte("R"), 3000)
	write(t, filepath.Join(root, "photos", "a.jpg"), real, now)
	write(t, filepath.Join(root, "photos", "backup", "a.jpg"), real, now)
	gs, err := storage.FindDuplicates(context.Background(), []string{root}, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 1 || len(gs[0].Files) != 2 {
		t.Fatalf("only the real copies are duplicates, not build output: %+v", gs)
	}
}
