package catalog_test

import (
	"testing"
	"testing/fstest"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Apps) < 50 || len(cat.Profiles) < 10 {
		t.Fatalf("catalog suspiciously small: %d apps, %d profiles", len(cat.Apps), len(cat.Profiles))
	}
}

func TestDevProfilesResolveWithDependenciesFirst(t *testing.T) {
	cat, _ := catalog.Load(catalogdata.FS)
	res, err := cat.Resolve("dev-rust")
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, a := range res.Apps {
		pos[a.ID] = i
	}
	if pos["vs-build-tools-2022"] > pos["rustup"] {
		t.Fatalf("rustup must come after the MSVC build tools it links with: %v", pos)
	}
	if _, ok := pos["git"]; !ok {
		t.Fatal("dev-rust should inherit git from dev-base")
	}
	found := false
	for _, r := range res.Recipes {
		found = found || r == "rust-stable"
	}
	if !found {
		t.Fatalf("app-attached recipe rust-stable missing: %v", res.Recipes)
	}
}

func fsOf(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

const okApp = `- {id: a, name: A, category: c, description: d, homepage: "https://a.example", winget: X.A, publisher: P}
`

func TestLoadRejectsBadCatalogs(t *testing.T) {
	cases := map[string]map[string]string{
		"unknown field": {"apps/a.yml": okApp + "  wingett: typo\n", "profiles/p.yml": "[]", "recipes/r.yml": "[]"},
		"duplicate winget": {"apps/a.yml": okApp + `- {id: b, name: B, category: c, description: d, homepage: "https://b.example", winget: x.a, publisher: P}
`, "profiles/p.yml": "[]", "recipes/r.yml": "[]"},
		"http homepage": {"apps/a.yml": `- {id: a, name: A, category: c, description: d, homepage: "http://a.example", winget: X.A, publisher: P}
`, "profiles/p.yml": "[]", "recipes/r.yml": "[]"},
		"profile unknown app": {"apps/a.yml": okApp, "profiles/p.yml": "- {id: p, name: P, apps: [{id: zzz}]}\n", "recipes/r.yml": "[]"},
		"extends cycle":       {"apps/a.yml": okApp, "profiles/p.yml": "- {id: p, name: P, extends: [q]}\n- {id: q, name: Q, extends: [p]}\n", "recipes/r.yml": "[]"},
		"requires cycle": {"apps/a.yml": `- {id: a, name: A, category: c, description: d, homepage: "https://a.example", winget: X.A, publisher: P, requires: [b]}
- {id: b, name: B, category: c, description: d, homepage: "https://b.example", winget: X.B, publisher: P, requires: [a]}
`, "profiles/p.yml": "- {id: p, name: P, apps: [{id: a}]}\n", "recipes/r.yml": "[]"},
		"unknown recipe": {"apps/a.yml": okApp, "profiles/p.yml": "- {id: p, name: P, recipes: [nope]}\n", "recipes/r.yml": "[]"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := catalog.Load(fsOf(files)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
