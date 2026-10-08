package catalog_test

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestTaglinesAreAttachedWithoutJoiningThePopularList(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	dolphin := cat.Apps["dolphin"]
	if dolphin == nil || dolphin.Tagline == nil || dolphin.Tagline.ES == "" {
		t.Fatalf("dolphin should have a Spanish tagline: %+v", dolphin)
	}
	for _, id := range cat.Featured {
		if id == "dolphin" {
			t.Fatal("a tagline must not make an app popular")
		}
	}
}

func TestEveryGamingAppHasATagline(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	var lacking []string
	for id, a := range cat.Apps {
		if (strings.HasPrefix(a.Category, "gaming/emulators") || strings.HasPrefix(a.Category, "gaming/mods") || strings.HasPrefix(a.Category, "gaming/engines")) && a.Tagline == nil {
			lacking = append(lacking, id)
		}
	}
	// A handful of niche tools may go without; most of the section must read well on a card.
	if len(lacking) > 15 {
		t.Fatalf("%d emulator, mod and engine apps have no tagline: %v", len(lacking), lacking)
	}
}

func TestTaglinesRejectBadEntries(t *testing.T) {
	base := map[string]string{"apps/a.yml": okApp, "profiles/p.yml": "[]", "recipes/r.yml": "[]", "detect/d.yml": "[]"}
	cases := map[string][2]string{
		"unknown app":      {"", "- {id: zzz, en: x, es: y}\n"},
		"missing es":       {"", "- {id: a, en: x, es: \"\"}\n"},
		"duplicate":        {"", "- {id: a, en: x, es: y}\n- {id: a, en: x, es: y}\n"},
		"too long":         {"", "- {id: a, en: \"" + strings.Repeat("x", 61) + "\", es: y}\n"},
		"also in featured": {"- {id: a, en: x, es: y}\n", "- {id: a, en: x, es: y}\n"},
	}
	for name, c := range cases {
		files := map[string]string{"featured.yml": c[0], "taglines.yml": c[1]}
		if c[0] == "" {
			files["featured.yml"] = "[]"
		}
		for k, v := range base {
			files[k] = v
		}
		if _, err := catalog.Load(fsOf(files)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
