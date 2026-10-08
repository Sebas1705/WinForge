package catalog_test

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestEmulationDataLoadsAndIsConsistent(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	e := cat.Emulation
	if e == nil || len(e.Systems) < 25 || len(e.Emulators) < 40 || len(e.Sources) < 40 {
		t.Fatalf("emulation data is missing or small: %+v", e)
	}
	// Every system with sources has at least one emulator that plays it. Systems
	// without a source of their own (Switch, Xbox...) show the general ones.
	played := map[string]bool{}
	for _, systems := range e.Emulators {
		for _, s := range systems {
			played[s] = true
		}
	}
	has := map[string]int{}
	for _, s := range e.Sources {
		has[s.System]++
	}
	for _, s := range e.Systems {
		if has[s.ID] > 0 && !played[s.ID] {
			t.Errorf("system %s has sources but no emulator plays it", s.ID)
		}
	}
	// Emulators are catalog emulators (or RetroArch and the Doom engines).
	for id := range e.Emulators {
		a := cat.Apps[id]
		if !strings.HasPrefix(a.Category, "gaming/") {
			t.Errorf("%s is not a gaming app (%s)", id, a.Category)
		}
	}
	// Patches, translations and randomizers need the person's own game and say so.
	for _, s := range e.Sources {
		if (s.Kind == "hacks" || s.Kind == "translations" || s.Kind == "randomizers") && !s.Base && s.ID != "idgames" {
			t.Errorf("%s changes an existing game, so it must say it needs the original (base: true)", s.ID)
		}
	}
}

func TestNoSourceLinksToASiteThatSharesGamesWithoutPermission(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range cat.Emulation.Sources {
		for _, bad := range []string{"emuparadise", "romsmania", "coolrom", "vimm", "cdromance", "romspure", "romsgames"} {
			if strings.Contains(strings.ToLower(s.URL), bad) {
				t.Errorf("%s links to %s", s.ID, s.URL)
			}
		}
	}
}

func TestEmulationRejectsBadEntries(t *testing.T) {
	base := map[string]string{"apps/a.yml": okApp, "profiles/p.yml": "[]", "recipes/r.yml": "[]", "detect/d.yml": "[]"}
	head := "systems:\n  - {id: nes, en: NES, es: NES}\nemulators:\n  a: [nes]\n"
	src := func(line string) string { return head + "sources:\n  - " + line + "\n" }
	good := "{id: s1, system: nes, kind: homebrew, name: N, en: x, es: y, url: \"https://example.org/\"}"
	cases := map[string]string{
		"unknown emulator": "systems:\n  - {id: nes, en: NES, es: NES}\nemulators:\n  zzz: [nes]\nsources: []\n",
		"unknown system":   "systems:\n  - {id: nes, en: NES, es: NES}\nemulators:\n  a: [snes]\nsources: []\n",
		"no system":        "systems:\n  - {id: nes, en: NES, es: NES}\nemulators:\n  a: []\nsources: []\n",
		"http url":         src(strings.Replace(good, "https://", "http://", 1)),
		"unknown kind":     src(strings.Replace(good, "homebrew", "roms", 1)),
		"unknown source":   src(strings.Replace(good, "system: nes", "system: snes", 1)),
		"missing es":       src(strings.Replace(good, "es: y", "es: \"\"", 1)),
		"piracy site":      src(strings.Replace(good, "example.org", "www.emuparadise.me", 1)),
		"too long":         src(strings.Replace(good, "en: x", "en: \""+strings.Repeat("x", 141)+"\"", 1)),
		"duplicate":        head + "sources:\n  - " + good + "\n  - " + good + "\n",
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
	// And the good one loads.
	files := map[string]string{"emulation.yml": src(good)}
	for k, v := range base {
		files[k] = v
	}
	if _, err := catalog.Load(fsOf(files)); err != nil {
		t.Fatalf("a valid file must load: %v", err)
	}
}
