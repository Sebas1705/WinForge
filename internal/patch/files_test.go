package patch_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/patch"
)

func ipsFor(offset int, b ...byte) []byte {
	p := []byte("PATCH")
	p = append(p, byte(offset>>16), byte(offset>>8), byte(offset), 0, byte(len(b)))
	p = append(p, b...)
	return append(p, "EOF"...)
}

func TestApplyFilesNeverTouchesOrOverwrites(t *testing.T) {
	dir := t.TempDir()
	rom := game(64)
	romPath := filepath.Join(dir, "My Game.sfc")
	patchPath := filepath.Join(dir, "hack.ips")
	if err := os.WriteFile(romPath, rom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patchPath, ipsFor(3, 9, 9), 0o644); err != nil {
		t.Fatal(err)
	}

	out1, info, err := patch.ApplyFiles(romPath, patchPath)
	if err != nil || filepath.Base(out1) != "My Game (patched).sfc" || info.Format != "IPS" {
		t.Fatalf("%v %q %+v", err, out1, info)
	}
	got, _ := os.ReadFile(out1)
	if got[3] != 9 || got[4] != 9 || len(got) != 64 {
		t.Fatalf("not patched: %x", got[:8])
	}
	if orig, _ := os.ReadFile(romPath); !bytes.Equal(orig, rom) {
		t.Fatal("the original game was changed")
	}
	out2, _, err := patch.ApplyFiles(romPath, patchPath)
	if err != nil || filepath.Base(out2) != "My Game (patched 2).sfc" {
		t.Fatalf("a second run must not overwrite the first: %v %q", err, out2)
	}
	if again, _ := os.ReadFile(out1); !bytes.Equal(again, got) {
		t.Fatal("the first patched file was overwritten")
	}
}

func TestApplyFilesRefusesWhatItCannotDo(t *testing.T) {
	dir := t.TempDir()
	w := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rom := w("g.gba", game(32))
	zipped := w("g.zip", []byte("PK\x03\x04rest"))
	good := w("p.ips", ipsFor(0, 1))
	for name, c := range map[string][2]string{
		"zipped game":  {zipped, good},
		"not a patch":  {rom, w("x.txt", []byte("hello"))},
		"missing game": {filepath.Join(dir, "nope.gba"), good},
		"directory":    {dir, good},
	} {
		if out, _, err := patch.ApplyFiles(c[0], c[1]); err == nil {
			t.Errorf("%s: expected an error, wrote %s", name, out)
		}
	}
	// Nothing was written by the failures.
	left, _ := filepath.Glob(filepath.Join(dir, "*patched*"))
	if len(left) != 0 {
		t.Fatalf("failed runs left files behind: %v", left)
	}
	_, _, err := patch.ApplyFiles(zipped, good)
	if err == nil || !strings.Contains(err.Error(), "extract") {
		t.Fatalf("the zip error should say what to do: %v", err)
	}
}
