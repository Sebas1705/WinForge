package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Sebas1705/WinForge/internal/diag"
)

func diagTestApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	a.diag.env = func() diag.Input {
		return diag.Input{
			Version: "1.0.0", Build: "go test", Windows: diag.Windows{Version: "10.0.26100", Build: "26100.1", Edition: "Pro", Arch: "amd64"},
			Language: "es", Admin: false, WingetVersion: "v1.9", CatalogApps: 3, CatalogProfiles: 1,
			DataFS:  fstest.MapFS{"history.json": {Data: []byte("SECRET")}, "note-for-ana.log": {Data: []byte("opened C:\\Users\\ana\\a.txt\n")}},
			DataDir: `C:\Users\ana\AppData\Roaming\WinForge`,
			Users:   []string{"ana"}, Homes: []string{`C:\Users\ana`},
			Scans: []diag.Scan{{Name: "apps", Counts: map[string]int{"installed": 2, "catalog": 3}}},
		}
	}
	return a
}

func TestDiagSaveWritesExactlyWhatThePreviewShowed(t *testing.T) {
	a := diagTestApp(t)
	preview, err := a.DiagPreview()
	if err != nil {
		t.Fatal(err)
	}
	// the world changes between the preview and the save: the saved file must still be the preview
	a.diag.env = func() diag.Input { return diag.Input{Version: "changed"} }
	path := filepath.Join(t.TempDir(), "report.json")
	got, err := a.DiagSave(path)
	if err != nil || got != path {
		t.Fatalf("%q %v", got, err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != preview {
		t.Errorf("saved file differs from the preview:\n%s\n---\n%s", b, preview)
	}
	for _, bad := range []string{"ana", "SECRET", "a.txt"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("the saved report contains %q", bad)
		}
	}
}

func TestDiagSaveAsksForAPathAndCancelWritesNothing(t *testing.T) {
	a := diagTestApp(t)
	dir := t.TempDir()
	a.diag.pick = func(name string) (string, error) {
		if name != "winforge-diagnostics.json" {
			t.Errorf("default name %q", name)
		}
		return "", nil // the person cancelled
	}
	if p, err := a.DiagSave(""); p != "" || err != nil {
		t.Errorf("cancel: %q %v", p, err)
	}
	target := filepath.Join(dir, "chosen.txt")
	a.diag.pick = func(string) (string, error) { return target, nil }
	if p, err := a.DiagSave(""); p != target || err != nil {
		t.Fatalf("%q %v", p, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("not written")
	}
}

func TestDiagSaveRefusesAnythingButAReportFile(t *testing.T) {
	a := diagTestApp(t)
	dir := t.TempDir()
	for _, p := range []string{"report.json", filepath.Join(dir, "x.exe"), filepath.Join(dir, "x.ps1"), filepath.Join(dir, "noext"), dir, filepath.Join(dir, "sub.json") + string(os.PathSeparator)} {
		if _, err := a.DiagSave(p); err == nil || !strings.HasPrefix(err.Error(), "diag:path") {
			t.Errorf("%q: %v", p, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DiagSave(filepath.Join(dir, "folder.json")); err == nil {
		t.Error("a folder was accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("something was written: %v", entries)
	}
}

func TestDiagPreviewOfTheRealMachineIsValidAndScrubbed(t *testing.T) {
	a := testApp(t) // the real collectors: this PC
	text, err := a.DiagPreview()
	if err != nil || !strings.Contains(text, `"kind": "winforge-diagnostic-report"`) {
		t.Fatalf("%v\n%s", err, text)
	}
	users, homes := diag.Identity()
	for _, u := range users {
		if len(u) >= 4 && strings.Contains(strings.ToLower(text), strings.ToLower(u)) && !strings.Contains(strings.ToLower(u), " ") {
			t.Errorf("the report of this PC contains the user name %q", u)
		}
	}
	for _, h := range homes {
		if len(h) > 3 && strings.Contains(strings.ToLower(text), strings.ToLower(strings.ReplaceAll(h, `\`, `\\`))) {
			t.Errorf("the report of this PC contains the home folder %q", h)
		}
	}
}
