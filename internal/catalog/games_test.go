package catalog_test

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestInstallableGamesAreWellFormedAndPlayable(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Games) < 15 {
		t.Fatalf("only %d installable games", len(cat.Games))
	}
	played := map[string]bool{}
	for _, systems := range cat.Emulation.Emulators {
		for _, s := range systems {
			played[s] = true
		}
	}
	urls := map[string]bool{}
	for _, g := range cat.Games {
		if !played[g.System] {
			t.Errorf("%s: no emulator plays %s", g.ID, g.System)
		}
		if urls[g.URL] {
			t.Errorf("%s: URL listed twice", g.ID)
		}
		urls[g.URL] = true
		if !strings.Contains(g.License, "Freeware") && !strings.Contains(g.License, "Public domain") && !strings.Contains(g.License, "BSD") && g.License != "zlib" {
			t.Errorf("%s: license %q is not one that allows free redistribution", g.ID, g.License)
		}
	}
}

func TestGamesRejectBadEntries(t *testing.T) {
	base := map[string]string{
		"apps/a.yml": okApp, "profiles/p.yml": "[]", "recipes/r.yml": "[]", "detect/d.yml": "[]",
		"emulation.yml": "systems:\n  - {id: gb, en: GB, es: GB}\nemulators:\n  a: [gb]\nsources: []\n",
	}
	sha := strings.Repeat("a", 64)
	good := "{id: g, name: G, system: gb, en: x, es: y, license: zlib, homepage: \"https://github.com/x/y\", url: \"https://github.com/x/y/releases/download/v1/g.gb\", sha256: " + sha + ", size: 10, kind: file, file: g.gb, entry: g.gb}"
	games := func(l string) string { return "games:\n  - " + l + "\n" }
	cases := map[string]string{
		"http url":       strings.Replace(good, "url: \"https://github.com/x/y/releases", "url: \"http://github.com/x/y/releases", 1),
		"unknown host":   strings.Replace(good, "url: \"https://github.com/x/y/releases/download/v1/g.gb", "url: \"https://example.org/g.gb", 1),
		"piracy site":    strings.Replace(good, "url: \"https://github.com/x/y/releases/download/v1/g.gb", "url: \"https://www.emuparadise.me/g.gb", 1),
		"short hash":     strings.Replace(good, sha, "abc", 1),
		"upper hash":     strings.Replace(good, sha, strings.Repeat("A", 64), 1),
		"unknown system": strings.Replace(good, "system: gb", "system: nes", 1),
		"no license":     strings.Replace(good, "license: zlib, ", "license: \"\", ", 1),
		"zero size":      strings.Replace(good, "size: 10", "size: 0", 1),
		"huge size":      strings.Replace(good, "size: 10", "size: 9999999999", 1),
		"bad kind":       strings.Replace(good, "kind: file", "kind: exe", 1),
		"file with path": strings.Replace(strings.Replace(good, "file: g.gb", "file: ../g.gb", 1), "entry: g.gb", "entry: ../g.gb", 1),
		"entry escapes":  strings.Replace(strings.Replace(good, "kind: file, file: g.gb, entry: g.gb", "kind: zip, entry: ../g.gb", 1), "", "", 1),
		"entry other":    strings.Replace(good, "entry: g.gb", "entry: other.gb", 1),
		"http homepage":  strings.Replace(good, "homepage: \"https://github.com/x/y\"", "homepage: \"http://github.com/x/y\"", 1),
		"duplicate":      good + "\n  - " + good,
		"missing en":     strings.Replace(good, "en: x, ", "en: \"\", ", 1),
		"unknown field":  strings.Replace(good, "kind: file", "kind: file, run: evil.exe", 1),
		"windows entry":  strings.Replace(strings.Replace(good, "kind: file, file: g.gb, entry: g.gb", "kind: zip, entry: \"a\\\\b.gb\"", 1), "", "", 1),
	}
	for name, line := range cases {
		files := map[string]string{"games.yml": games(line)}
		for k, v := range base {
			files[k] = v
		}
		if _, err := catalog.Load(fsOf(files)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	files := map[string]string{"games.yml": games(good)}
	for k, v := range base {
		files[k] = v
	}
	if _, err := catalog.Load(fsOf(files)); err != nil {
		t.Fatalf("a valid game must load: %v", err)
	}
}

func TestRunSpecsAreValidated(t *testing.T) {
	base := map[string]string{"apps/a.yml": okApp, "profiles/p.yml": "[]", "recipes/r.yml": "[]", "detect/d.yml": "[]"}
	head := "systems:\n  - {id: gb, en: GB, es: GB}\nemulators:\n  a: [gb]\nsources: []\nrun:\n"
	cases := map[string]string{
		"not an emulator": head + "  zzz: {exes: [a.exe], hints: [a], args: [\"{file}\"]}\n",
		"no exes":         head + "  a: {exes: [], hints: [a], args: [\"{file}\"]}\n",
		"path in exe":     head + "  a: {exes: [\"C:/x/a.exe\"], hints: [a], args: [\"{file}\"]}\n",
		"not an exe":      head + "  a: {exes: [a.bat], hints: [a], args: [\"{file}\"]}\n",
		"no file arg":     head + "  a: {exes: [a.exe], hints: [a], args: [\"-x\"]}\n",
	}
	for name, body := range cases {
		files := map[string]string{"emulation.yml": body}
		for k, v := range base {
			files[k] = v
		}
		if _, err := catalog.Load(fsOf(files)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
