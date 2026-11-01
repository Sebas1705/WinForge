package profile_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func load(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestExportImportRoundTrip(t *testing.T) {
	cat := load(t)
	p := profile.FromInstalled("my-pc", "My PC", map[string]string{"git": "2.50.0", "vscode": "1.99"}, true)
	b, err := profile.Export(p)
	if err != nil {
		t.Fatal(err)
	}
	imp, err := profile.Import(b, cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Profile.Apps) != 2 || imp.Profile.Apps[0].ID != "git" || imp.Profile.Apps[0].Version != "2.50.0" {
		t.Fatalf("round trip lost data: %+v", imp.Profile.Apps)
	}
}

func TestImportDropsUnknownAndRejectsHostileFiles(t *testing.T) {
	cat := load(t)
	imp, err := profile.Import([]byte(`{"winforge":1,"profile":{"id":"x","name":"X",
		"apps":[{"id":"git"},{"id":"not-in-catalog"},{"id":"git"}],"recipes":["git-defaults","evil"]}}`), cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Profile.Apps) != 1 || imp.UnknownApps[0] != "not-in-catalog" || imp.UnknownRecipes[0] != "evil" {
		t.Fatalf("%+v", imp)
	}
	bad := map[string]string{
		"command field":   `{"winforge":1,"profile":{"id":"x","name":"X","powershell":"rm -rf /"}}`,
		"path traversal":  `{"winforge":1,"profile":{"id":"../x","name":"X"}}`,
		"newer format":    `{"winforge":99,"profile":{"id":"x","name":"X"}}`,
		"no version":      `{"profile":{"id":"x","name":"X"}}`,
		"unknown extends": `{"winforge":1,"profile":{"id":"x","name":"X","extends":["zzz"]}}`,
		"not json":        `hello`,
	}
	for name, body := range bad {
		if _, err := profile.Import([]byte(body), cat); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestStoreSaveListDelete(t *testing.T) {
	cat := load(t)
	s := &profile.Store{Dir: t.TempDir()}
	if err := s.Save(profile.FromInstalled("a", "Alpha", map[string]string{"git": ""}, false)); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(cat)
	if err != nil || len(list) != 1 || list[0].Name != "Alpha" {
		t.Fatalf("%v %v", list, err)
	}
	if err := s.Save(catalog.Profile{ID: "../evil", Name: "x"}); err == nil {
		t.Fatal("path traversal id must be rejected")
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(cat); len(list) != 0 {
		t.Fatal("not deleted")
	}
}

func TestWingetImportFormat(t *testing.T) {
	cat := load(t)
	b, err := profile.WingetImport([]catalog.ProfileApp{{ID: "git", Version: "2.50.0"}, {ID: "vscode"}}, cat)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sources []struct {
			Packages []struct{ PackageIdentifier, Version string }
		}
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	p := doc.Sources[0].Packages
	if len(p) != 2 || p[0].PackageIdentifier != "Git.Git" || p[0].Version != "2.50.0" || !strings.Contains(string(b), "winget-packages.schema") {
		t.Fatalf("%s", b)
	}
}

func TestImportRejectsLineBreaksAndOddVersions(t *testing.T) {
	cat := load(t)
	mk := func(name, version string) []byte {
		b, _ := json.Marshal(map[string]any{"winforge": 1, "profile": map[string]any{
			"id": "x", "name": name, "apps": []map[string]string{{"id": "git", "version": version}}}})
		return b
	}
	bad := map[string][2]string{
		"newline in name":   {"A\nInvoke-Evil", ""},
		"CR in name":        {"A\rB", ""},
		"U+2028 in name":    {"A\u2028B", ""},
		"NUL in name":       {"A\x00B", ""},
		"huge name":         {strings.Repeat("a", 500), ""},
		"newline version":   {"ok", "1.0\nInvoke-Evil"},
		"quote version":     {"ok", "1.0'; Invoke-Evil; '"},
		"backtick version":  {"ok", "1.0`n"},
		"subexpr version":   {"ok", "$(Invoke-Evil)"},
		"space version":     {"ok", "1.0 --override x"},
		"double quote":      {"ok", `1.0"`},
		"leading dash":      {"ok", "-1.0"},
		"semicolon version": {"ok", "1;2"},
	}
	for label, c := range bad {
		if _, err := profile.Import(mk(c[0], c[1]), cat); err == nil {
			t.Errorf("%s: expected an error", label)
		}
	}
	for _, v := range []string{"", "2.50.0", "1.99.0-beta.1", "2024.1+build5", "1:2.3"} {
		if _, err := profile.Import(mk("Fine, name (v2) 'quoted'", v), cat); err != nil {
			t.Errorf("version %q rejected: %v", v, err)
		}
	}
}
