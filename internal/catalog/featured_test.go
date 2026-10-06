package catalog_test

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestFeaturedTaglinesAreLoadedAndOrdered(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Featured) < 60 || cat.Featured[0] != "firefox" {
		t.Fatalf("featured: %d, first %q", len(cat.Featured), cat.Featured[0])
	}
	a := cat.Apps["firefox"]
	if a.Tagline == nil || a.Tagline.ES != "Navegador rápido y privado" {
		t.Fatalf("%+v", a.Tagline)
	}
	if cat.Apps["go"].Tagline == nil {
		t.Fatal("go should be featured")
	}
}

func TestFeaturedRejectsBadEntries(t *testing.T) {
	base := map[string]string{"apps/a.yml": okApp, "profiles/p.yml": "[]", "recipes/r.yml": "[]", "detect/d.yml": "[]"}
	cases := map[string]string{
		"unknown app": "- {id: zzz, en: x, es: y}\n",
		"missing es":  "- {id: a, en: x, es: \"\"}\n",
		"duplicate":   "- {id: a, en: x, es: y}\n- {id: a, en: x, es: y}\n",
		"too long":    "- {id: a, en: \"" + strings.Repeat("x", 61) + "\", es: y}\n",
	}
	for name, body := range cases {
		files := map[string]string{"featured.yml": body}
		for k, v := range base {
			files[k] = v
		}
		if _, err := catalog.Load(fsOf(files)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
