package emu_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/emu"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLocateFindsAnEmulatorByFolderHintAndExeName(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "mGBA", "mGBA.exe")
	touch(t, want)
	touch(t, filepath.Join(root, "Other", "mGBA.exe")) // right name, wrong folder
	touch(t, filepath.Join(root, "Dolphin", "Dolphin.exe"))
	spec := catalog.RunSpec{Exes: []string{"mGBA.exe"}, Hints: []string{"mgba"}}
	if got := emu.Locate(spec, []string{root}); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := emu.Locate(catalog.RunSpec{Exes: []string{"Nope.exe"}, Hints: []string{"mgba"}}, []string{root}); got != "" {
		t.Fatalf("a missing program must not be found: %q", got)
	}
}

func TestLocateHandlesWinGetPackageFoldersAndDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "Vendor.MGBA_Microsoft.Winget.Source_8wekyb3d8bbwe", "mGBA-0.10", "mGBA.exe")
	touch(t, deep)
	spec := catalog.RunSpec{Exes: []string{"mGBA.exe"}, Hints: []string{"mgba"}}
	// The hint is in the package folder's name; the program is one folder further down.
	if got := emu.Locate(spec, []string{root}); got != deep {
		t.Fatalf("got %q, want %q", got, deep)
	}
	tooDeep := t.TempDir()
	touch(t, filepath.Join(tooDeep, "a", "b", "c", "d", "mgba", "mGBA.exe"))
	if got := emu.Locate(spec, []string{tooDeep}); got != "" {
		t.Fatalf("the search must stop at a few levels: %q", got)
	}
	if got := emu.Locate(spec, []string{filepath.Join(root, "does-not-exist")}); got != "" {
		t.Fatalf("a missing root is not an error: %q", got)
	}
}

func TestArgsFillThePathsAndKeepEverythingElse(t *testing.T) {
	got := emu.Args([]string{"-b", "-e", "{file}", "--path={dir}", "x"}, `C:\Games\a b\g.gb`, `C:\Games\a b`)
	want := []string{"-b", "-e", `C:\Games\a b\g.gb`, `--path=C:\Games\a b`, "x"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}

func TestStartAndRegisterRunTheProgram(t *testing.T) {
	// The test binary runs itself as the "emulator".
	exe := os.Args[0]
	out := filepath.Join(t.TempDir(), "ran.txt")
	t.Setenv("WINFORGE_EMU_HELPER", out)
	if err := emu.Register(exe, []string{"-test.run=TestHelperProcess"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), "ran") {
		t.Fatalf("the program did not run: %q", b)
	}
	_ = os.Remove(out)
	if err := emu.Start(exe, []string{"-test.run=TestHelperProcess"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(out); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("the started program did not run")
	}
	if err := emu.Start(filepath.Join(t.TempDir(), "missing.exe"), nil); err == nil {
		t.Fatal("starting a missing program must fail")
	}
	if err := emu.Register(exe, nil); err == nil {
		t.Fatal("registering with no command must fail")
	}
}

func TestHelperProcess(t *testing.T) {
	if p := os.Getenv("WINFORGE_EMU_HELPER"); p != "" {
		_ = os.WriteFile(p, []byte("ran"), 0o644)
	}
}
