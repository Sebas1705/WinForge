package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/eol"
)

// These tests never touch the network: the API answers are fixtures.
const sample = `[
 {"cycle":"4","releaseDate":"2026-10-01","eol":false,"lts":false},
 {"cycle":"3","releaseDate":"2025-10-01","eol":"2030-10-31","lts":"2026-01-01"},
 {"cycle":"2","releaseDate":"2024-10-01","eol":"2026-06-30","lts":"2099-01-01"},
 {"cycle":"1.5","releaseDate":"2023-10-01","eol":"2024-01-01","lts":true},
 {"cycle":"1.10","releaseDate":"2023-10-01","eol":"2024-02-01","lts":false},
 {"cycle":"1.9","releaseDate":"2022-10-01","eol":"2021-12-31","lts":false},
 {"cycle":"0.9","releaseDate":"2010-01-01","eol":true,"lts":false},
 {"cycle":"0.8","releaseDate":"2023-01-01","eol":true,"lts":false},
 {"cycle":"9","releaseDate":"2027-01-01","eol":false,"lts":false}
]`

func TestBuildProductFiltersAndOrders(t *testing.T) {
	s := spec{Slug: "x", ID: "x", Name: "X", Apps: []string{"a"}, Install: map[string]string{"3": "a", "7": "b"}}
	p, warns, err := buildProduct(s, []byte(sample), "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range p.Cycles {
		got = append(got, c.Cycle)
	}
	// 9 is not released; 1.9 ended before the cutoff; 0.9 and 0.8 ended with no date.
	if strings.Join(got, " ") != "4 3 2 1.10 1.5" {
		t.Fatalf("cycles: %v", got)
	}
	byCycle := map[string]eol.Cycle{}
	for _, c := range p.Cycles {
		byCycle[c.Cycle] = c
	}
	if c := byCycle["4"]; c.EOL != "" || c.LTS {
		t.Errorf("a supported cycle has no end: %+v", c)
	}
	if c := byCycle["3"]; !c.LTS || c.App != "a" || c.EOL != "2030-10-31" {
		t.Errorf("an LTS date in the past means long-term now: %+v", c)
	}
	if c := byCycle["2"]; c.LTS {
		t.Errorf("an LTS date in the future is not long-term yet: %+v", c)
	}
	if !byCycle["1.5"].LTS {
		t.Error("lts: true")
	}
	if p.Source != "https://endoflife.date/x" {
		t.Errorf("source %q", p.Source)
	}
	joined := strings.Join(warns, "|")
	if !strings.Contains(joined, "x 0.8") || strings.Contains(joined, "x 0.9") {
		t.Errorf("only recent undated endings are worth a warning: %v", warns)
	}
	if !strings.Contains(joined, "install mapping for cycle 7") {
		t.Errorf("a mapping to a missing cycle is reported: %v", warns)
	}
}

func TestBuildProductOnlyAndErrors(t *testing.T) {
	s := spec{Slug: "x", ID: "x", Name: "X", Apps: []string{"a"}, Only: []string{"1.9"}}
	p, _, err := buildProduct(s, []byte(sample), "2026-11-17")
	if err != nil || len(p.Cycles) != 1 || p.Cycles[0].Cycle != "1.9" || p.Cycles[0].EOL != "2021-12-31" {
		t.Fatalf("an explicit cycle is kept even when old: %v %+v", err, p.Cycles)
	}
	s.Only = []string{"6.6"}
	if _, _, err := buildProduct(s, []byte(sample), "2026-11-17"); err == nil {
		t.Error("a cycle that left the source must be an error")
	}
	if _, _, err := buildProduct(s, []byte("not json"), "2026-11-17"); err == nil {
		t.Error("bad JSON must be an error")
	}
	bad := `[{"cycle":"1","releaseDate":"2024-01-01","eol":"someday"}]`
	if _, _, err := buildProduct(spec{Slug: "x", ID: "x"}, []byte(bad), "2026-11-17"); err == nil {
		t.Error("an eol that is neither a date nor a boolean must be an error")
	}
}

func TestLongTermLineWithoutADateIsLeftOut(t *testing.T) {
	body := `[{"cycle":"21","releaseDate":"2023-09-01","eol":false,"lts":true},{"cycle":"22","releaseDate":"2024-09-01","eol":false,"lts":false}]`
	p, warns, err := buildProduct(spec{Slug: "x", ID: "x"}, []byte(body), "2026-11-17")
	if err != nil || len(p.Cycles) != 1 || p.Cycles[0].Cycle != "22" || len(warns) != 1 {
		t.Fatalf("%v %+v %v", err, p.Cycles, warns)
	}
}

func fixtureFetch(t *testing.T) func(string) ([]byte, error) {
	return func(slug string) ([]byte, error) {
		if slug == "broken" {
			return nil, errors.New("HTTP 500")
		}
		return []byte(sample), nil
	}
}

func TestRenderIsDeterministicAndParses(t *testing.T) {
	ss := []spec{
		{Slug: "x", Vendor: "https://example.com/x", ID: "x", Name: "X", Apps: []string{"python-3-13"}, Follows: map[string]string{"powershell": "latest", "go": "latest"}, Install: map[string]string{"3": "python-3-13"}},
		{Slug: "y", ID: "y", Name: "Y (Java)", Scheme: "java", Apps: []string{"go"}},
	}
	d1, _, err := generate(ss, fixtureFetch(t), "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	a := render(d1, vendorsOf(ss))
	d2, _, _ := generate(ss, fixtureFetch(t), "2026-11-17")
	if !bytes.Equal(a, render(d2, vendorsOf(ss))) {
		t.Error("two runs on the same input differ")
	}
	back, err := eol.Parse(a)
	if err != nil {
		t.Fatalf("the output must parse: %v\n%s", err, a)
	}
	if back.AsOf != "2026-11-17" || len(back.Products) != 2 || back.Products[1].Scheme != "java" || back.Products[0].Follows["go"] != "latest" {
		t.Errorf("%+v", back)
	}
	if len(back.Products[0].Cycles) != len(d1.Products[0].Cycles) || back.Products[0].Cycles[1].App != "python-3-13" {
		t.Errorf("cycles do not round-trip: %+v", back.Products[0].Cycles)
	}
	text := string(a)
	if !strings.Contains(text, "# X: https://endoflife.date/x (vendor: https://example.com/x)") || !strings.Contains(text, "follows: {go: latest, powershell: latest}") {
		t.Errorf("comments or sorted follows missing:\n%s", text)
	}
	if _, _, err := generate([]spec{{Slug: "broken", ID: "b"}}, fixtureFetch(t), "2026-11-17"); err == nil {
		t.Error("a failed fetch must fail the run")
	}
}

func TestDiffLines(t *testing.T) {
	if d := diffLines("a\nb\nc", "a\nb\nc"); len(d) != 0 {
		t.Errorf("%v", d)
	}
	d := diffLines("a\nb\nc", "a\nx\nc")
	if strings.Join(d, "|") != "- b|+ x" {
		t.Errorf("%v", d)
	}
	if dropAsOf("asOf: \"1\"\nx\n") != "x\n" {
		t.Error("asOf line must be ignored by -check")
	}
}

// The product list must only name catalog apps that exist, so the tool cannot write a file that
// the app (or its own validation) would then reject.
func TestSpecsReferenceCatalogApps(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.ID] {
			t.Errorf("product %s listed twice", s.ID)
		}
		seen[s.ID] = true
		ids := append([]string{}, s.Apps...)
		for id := range s.Follows {
			ids = append(ids, id)
		}
		for _, id := range s.Install {
			ids = append(ids, id)
		}
		for _, id := range ids {
			if cat.Apps[id] == nil {
				t.Errorf("%s: %q is not a catalog app", s.ID, id)
			}
		}
	}
}

// Running the tool on local files (no network) writes a file; -check then agrees with it.
func TestRunWithAFolderAndCheck(t *testing.T) {
	dir := t.TempDir()
	for _, s := range specs {
		// A body every spec can digest: each of its cycles, so `Only` lists resolve.
		body := `[{"cycle":"1","releaseDate":"2024-01-01","eol":"2030-01-01"}`
		for _, c := range s.Only {
			body += `,{"cycle":"` + c + `","releaseDate":"2024-01-01","eol":"2030-01-01"}`
		}
		body += `]`
		if err := os.WriteFile(filepath.Join(dir, s.Slug+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "eol.yml")
	var so, se bytes.Buffer
	if code := run([]string{"-dir", dir, "-out", out, "-asof", "2026-11-17"}, &so, &se); code != 0 {
		t.Fatalf("exit %d: %s", code, se.String())
	}
	so.Reset()
	if code := run([]string{"-dir", dir, "-out", out, "-asof", "2030-01-01", "-check"}, &so, &se); code != 0 || !strings.Contains(so.String(), "matches") {
		t.Fatalf("check on a fresh file: exit %d: %s %s", code, so.String(), se.String())
	}
	b, _ := os.ReadFile(out)
	if err := os.WriteFile(out, []byte(strings.Replace(string(b), `"2030-01-01"`, `"2031-01-01"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	so.Reset()
	if code := run([]string{"-dir", dir, "-out", out, "-check"}, &so, &se); code != 1 || !strings.Contains(so.String(), "differs") {
		t.Fatalf("check on an edited file: exit %d: %s", code, so.String())
	}
	if code := run([]string{"-asof", "yesterday"}, &so, &se); code != 2 {
		t.Errorf("a bad -asof is a usage error, got %d", code)
	}
}
