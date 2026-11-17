package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/compare"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func writeProfile(t *testing.T, name string, p catalog.Profile) string {
	t.Helper()
	b, err := profile.Export(p)
	if err != nil {
		t.Fatal(err)
	}
	return writePicked(t, name, b)
}

func writePicked(t *testing.T, name string, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	rememberPicked(path)
	return path
}

func codeOf(err error) string {
	var e *compare.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "other: " + err.Error()
}

func TestCompareFileGroupsAppsAndTouchesNothing(t *testing.T) {
	a := testApp(t)
	a.installed = inst("git", "2.1", "7zip", "23", "vlc", "3")
	path := writeProfile(t, "other.winforge.json", catalog.Profile{ID: "other", Name: "Laptop", Apps: []catalog.ProfileApp{{ID: "git", Version: "2.0"}, {ID: "vlc", Version: "3"}, {ID: "ghost"}}})
	r, err := a.CompareFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.OnlyA) != 1 || r.OnlyA[0] != (CompareApp{ID: "7zip", Name: "7-Zip"}) {
		t.Fatalf("only here: %+v", r.OnlyA)
	}
	if len(r.OnlyB) != 0 || len(r.Both) != 1 || r.Both[0].ID != "vlc" {
		t.Fatalf("%+v", r)
	}
	if len(r.Differ) != 1 || r.Differ[0] != (CompareVersion{ID: "git", Name: "Git", A: "2.1", B: "2.0"}) {
		t.Fatalf("differ: %+v", r.Differ)
	}
	if len(r.Unknown) != 1 || r.Unknown[0] != (CompareUnknown{ID: "ghost", In: "b"}) {
		t.Fatalf("unknown: %+v", r.Unknown)
	}
	if r.A.Kind != "pc" || r.B.Kind != "file" || r.B.Name != "Laptop" || r.B.File != "other.winforge.json" || !r.B.Versioned || r.B.Apps != 2 || r.B.Profile == nil || r.A.Profile != nil {
		t.Fatalf("%+v %+v", r.A, r.B)
	}
	if list, _ := a.store.List(a.cat); len(list) != 0 {
		t.Fatal("comparing must not save a profile")
	}
}

func TestCompareNeedsAScan(t *testing.T) {
	a := testApp(t)
	path := writeProfile(t, "x.json", catalog.Profile{ID: "x", Name: "X"})
	if _, err := a.CompareFile(path); codeOf(err) != "no-scan" {
		t.Fatal(err)
	}
	if _, err := a.CompareCode("WF1.x"); codeOf(err) != "no-scan" {
		t.Fatal(err)
	}
}

func TestCompareOnlyReadsFilesThePersonPicked(t *testing.T) {
	a := testApp(t)
	a.installed = inst("git", "1")
	dir := t.TempDir()
	stray := filepath.Join(dir, "never-picked.json")
	os.WriteFile(stray, []byte(`{"winforge":1,"profile":{}}`), 0o644)
	for _, p := range []string{stray, "", filepath.Join(dir, "..", "secret.json"), `C:\Windows\win.ini`} {
		if _, err := a.CompareFile(p); codeOf(err) != "not-picked" {
			t.Errorf("%q: %v", p, err)
		}
	}
	if _, err := a.CompareFiles(stray, stray); codeOf(err) != "not-picked" {
		t.Fatal(err)
	}
}

func TestCompareRefusesFoldersHugeAndDamagedFiles(t *testing.T) {
	a := testApp(t)
	a.installed = inst("git", "1")
	dir := t.TempDir()
	rememberPicked(dir)
	if _, err := a.CompareFile(dir); codeOf(err) != "unreadable" {
		t.Fatalf("a folder: %v", err)
	}
	missing := filepath.Join(dir, "gone.json")
	rememberPicked(missing)
	if _, err := a.CompareFile(missing); codeOf(err) != "unreadable" {
		t.Fatalf("a missing file: %v", err)
	}
	huge := writePicked(t, "huge.json", []byte(strings.Repeat(" ", compare.MaxFile+10)))
	if _, err := a.CompareFile(huge); codeOf(err) != "too-big" {
		t.Fatalf("a huge file: %v", err)
	}
	for name, body := range map[string]string{"bad.json": "{not json", "bin.json": "\x00\xff\x00", "other.json": `{"hello":"world"}`} {
		if _, err := a.CompareFile(writePicked(t, name, []byte(body))); err == nil || !strings.HasPrefix(err.Error(), "compare:") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestCompareCodeAgainstThisPC(t *testing.T) {
	a := testApp(t)
	a.installed = inst("git", "2", "vlc", "3")
	code, err := profile.EncodeCode(catalog.Profile{ID: "c", Name: "Friend", Apps: []catalog.ProfileApp{{ID: "git", Version: "2"}, {ID: "7zip"}}, Recipes: []string{"nope"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.CompareCode(code)
	if err != nil {
		t.Fatal(err)
	}
	if r.B.Kind != "code" || r.B.Name != "Friend" || len(r.OnlyA) != 1 || r.OnlyA[0].ID != "vlc" || len(r.OnlyB) != 1 || r.OnlyB[0].ID != "7zip" || len(r.Both) != 1 {
		t.Fatalf("%+v", r)
	}
	if len(r.B.UnknownRecipes) != 1 {
		t.Fatalf("%+v", r.B)
	}
	if _, err := a.CompareCode("nonsense"); codeOf(err) != "not-code" {
		t.Fatal(err)
	}
}

func TestCompareTwoFiles(t *testing.T) {
	a := testApp(t) // never scanned: two files need no scan
	pa := writeProfile(t, "a.json", catalog.Profile{ID: "a", Name: "Desktop", Apps: []catalog.ProfileApp{{ID: "git", Version: "1"}, {ID: "vlc"}, {ID: "ghost"}}})
	pb := writeProfile(t, "b.json", catalog.Profile{ID: "b", Name: "Laptop", Apps: []catalog.ProfileApp{{ID: "git", Version: "2"}, {ID: "7zip"}, {ID: "ghost"}}})
	r, err := a.CompareFiles(pa, pb)
	if err != nil {
		t.Fatal(err)
	}
	if r.A.Kind != "file" || r.A.Name != "Desktop" || r.B.Name != "Laptop" || r.A.Profile == nil || r.B.Profile == nil {
		t.Fatalf("%+v %+v", r.A, r.B)
	}
	if len(r.OnlyA) != 1 || r.OnlyA[0].ID != "vlc" || len(r.OnlyB) != 1 || r.OnlyB[0].ID != "7zip" || len(r.Differ) != 1 {
		t.Fatalf("%+v", r)
	}
	if len(r.Unknown) != 1 || r.Unknown[0] != (CompareUnknown{ID: "ghost", In: "both"}) {
		t.Fatalf("%+v", r.Unknown)
	}
	if r.Both == nil || r.Unknown == nil || r.A.UnknownRecipes == nil {
		t.Fatal("empty lists must be [] in JSON, not null")
	}
}
