package diag

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const (
	planted = "sebss"
	home    = `C:\Users\sebss`
)

func plantedInput() Input {
	when := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	logLines := []string{
		`2026/11/01 10:00:00 opened C:\Users\sebss\Documents\secret.docx`,
		`2026/11/01 10:00:01 config at C:/Users/sebss/AppData/Roaming/WinForge/config.json`,
		`2026/11/01 10:00:02 json-escaped C:\\Users\\sebss\\Desktop`,
		`2026/11/01 10:00:03 user SEBSS logged in as DESKTOP\sebss`,
		`2026/11/01 10:00:04 other account C:\Users\otherperson\Pictures`,
		`2026/11/01 10:00:05 long-path \\?\C:\Users\sebss\x`,
	}
	return Input{
		Version:         "1.2.3-sebss",
		Build:           `go1.26 windows/amd64 built in C:\Users\sebss\src`,
		Windows:         Windows{Version: "10.0.26100", Build: "26100.4351 sebss", Edition: "Professional", Arch: "amd64"},
		Language:        "es-sebss",
		Admin:           true,
		WingetVersion:   `v1.9.0 (C:\Users\sebss\AppData\Local\Microsoft\WindowsApps)`,
		CatalogApps:     500,
		CatalogProfiles: 12,
		DataFS: fstest.MapFS{
			"history.json":                    {Data: []byte("SECRET-CONTENT-OF-HISTORY"), ModTime: when},
			"sebss-profile.json":              {Data: []byte("SECRET-CONTENT-OF-PROFILE"), ModTime: when},
			`C:\Users\sebss.json`:             {Data: []byte("x"), ModTime: when},
			"auto-updates/runs/20261101.json": {Data: []byte("SECRET-RUN"), ModTime: when},
			"a/b/c/d/too-deep.json":           {Data: []byte("x"), ModTime: when},
			"winforge.log":                    {Data: []byte(strings.Join(logLines, "\n") + "\n"), ModTime: when},
		},
		Scans: []Scan{
			{Name: "apps sebss", Counts: map[string]int{"installed": 40, "catalog": 500, "sebss": 1}},
			{Name: "updates", Counts: map[string]int{"available": 3}},
		},
		DataDir: `C:\Users\sebss\AppData\Roaming\WinForge`,
		Users:   []string{"sebss", `DESKTOP\sebss`},
		Homes:   []string{home, `C:\Users\sebss\`},
	}
}

func TestScrubbingPlantedUserNameAndHomeNeverAppear(t *testing.T) {
	in := plantedInput()
	text := Render(in)
	low := strings.ToLower(text)
	for _, bad := range []string{planted, `users\sebss`, `users\\sebss`, `users/sebss`, "otherperson", "secret.docx", "desktop"} {
		if strings.Contains(low, bad) {
			t.Errorf("the report contains %q:\n%s", bad, text)
		}
	}
	if !strings.Contains(text, "%USERPROFILE%") || !strings.Contains(text, "<user>") {
		t.Errorf("expected the placeholders in:\n%s", text)
	}
	// the parsed values too (not only the escaped text)
	var r Report
	if err := json.Unmarshal([]byte(text), &r); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Log.Lines, "\n")
	for _, want := range []string{`%AppData%\WinForge/config.json`, "opened <path>", "other account <path>"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log lines lack %q:\n%s", want, joined)
		}
	}
	flat := fmt.Sprintf("%+v %+v", r, *r.Log)
	if strings.Contains(strings.ToLower(flat), planted) {
		t.Errorf("a parsed value contains the user name: %s", flat)
	}
}

func TestScrubberEdgeCases(t *testing.T) {
	sc := NewScrubber([]string{"al", "Ana.Garcia"}, []string{`C:\Users\Ana.Garcia`, ""})
	for in, want := range map[string]string{
		`Local settings`:                   `Local settings`, // "al" inside a word stays
		`user al is here`:                  `user <user> is here`,
		`AL.`:                              `<user>.`,
		`C:\Users\ana.garcia\x`:            `%USERPROFILE%\x`,
		`C:\Users\someone-else\x`:          `C:\Users\<user>\x`,
		`/home/bob/.config`:                `/home/<user>/.config`,
		`D:\Users\Zed`:                     `D:\Users\<user>`,
		`c:/users/ANA.GARCIA/AppData`:      `%USERPROFILE%/AppData`,
		`ana.garcia@example.com`:           `<user>@example.com`,
		`nothing to hide: C:\Windows\Temp`: `nothing to hide: C:\Windows\Temp`,
	} {
		if got := sc.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := NewScrubber(nil, nil).Clean("plain"); got != "plain" {
		t.Error(got)
	}
}

// allowed is the whitelist of the document: every key that may exist, nested.
var allowed = map[string]any{
	"schema": nil, "kind": nil, "language": nil, "administrator": nil, "dataFolder": nil,
	"winforge": map[string]any{"version": nil, "build": nil},
	"windows":  map[string]any{"version": nil, "build": nil, "edition": nil, "arch": nil},
	"winget":   map[string]any{"found": nil, "version": nil},
	"catalog":  map[string]any{"apps": nil, "profiles": nil},
	"files":    []any{map[string]any{"name": nil, "bytes": nil, "modified": nil}},
	"log":      map[string]any{"file": nil, "lines": nil},
	"scans":    []any{map[string]any{"name": nil, "counts": map[string]any{"*": nil}}},
}

func checkKeys(t *testing.T, path string, got any, want any) {
	t.Helper()
	switch g := got.(type) {
	case map[string]any:
		w, ok := want.(map[string]any)
		if !ok {
			return // a leaf holding an object: only counts maps do this, covered by "*"
		}
		for k, v := range g {
			sub, ok := w[k]
			if !ok {
				if star, isStar := w["*"]; isStar {
					sub = star
				} else {
					t.Errorf("the report has a field outside the whitelist: %s.%s", path, k)
					continue
				}
			}
			checkKeys(t, path+"."+k, v, sub)
		}
	case []any:
		if w, ok := want.([]any); ok && len(w) == 1 {
			for i, v := range g {
				checkKeys(t, fmt.Sprintf("%s[%d]", path, i), v, w[0])
			}
		}
	}
}

func TestReportContainsOnlyWhitelistedFieldsAndNoFileContents(t *testing.T) {
	text := Render(plantedInput())
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatal(err)
	}
	checkKeys(t, "$", m, allowed)
	for k := range allowed {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	for _, never := range []string{"SECRET-CONTENT", "SECRET-RUN", "too-deep", "installed software", "PATH=", "HKEY_"} {
		if strings.Contains(text, never) {
			t.Errorf("the report must not contain %q", never)
		}
	}
	// the Input type has no field that could carry them: a compile-time-ish guard on the document type
	if _, ok := m["installed"]; ok {
		t.Error("installed software list")
	}
}

func TestFilesAreNamesSizesAndDatesOnly(t *testing.T) {
	r := Build(plantedInput())
	var names []string
	for _, f := range r.Files {
		names = append(names, f.Name)
		if f.Bytes <= 0 || f.Modified != "2026-11-01T10:00:00Z" {
			t.Errorf("%+v", f)
		}
	}
	got := strings.Join(names, ",")
	for _, want := range []string{"auto-updates/runs/20261101.json", "history.json", "winforge.log", "<user>-profile.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing in %s", want, got)
		}
	}
	if strings.Contains(got, "too-deep") {
		t.Error("the walk must stop at a few levels")
	}
	if r.DataFolder != `%AppData%\WinForge` {
		t.Errorf("the real path must not be shown: %q", r.DataFolder)
	}
}

func TestFileListIsCapped(t *testing.T) {
	m := fstest.MapFS{}
	for i := 0; i < MaxFiles+50; i++ {
		m[fmt.Sprintf("f%04d.json", i)] = &fstest.MapFile{Data: []byte("x")}
	}
	if n := len(Build(Input{DataFS: m}).Files); n != MaxFiles {
		t.Errorf("%d files", n)
	}
}

func TestLogTailKeepsTheLastLinesAndCutsLongOnes(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 120; i++ {
		fmt.Fprintf(&b, "line %d\r\n", i)
	}
	b.WriteString(strings.Repeat("x", 1000) + "\n")
	m := fstest.MapFS{
		"old.log":  {Data: []byte("old"), ModTime: time.Unix(100, 0)},
		"main.LOG": {Data: []byte(b.String()), ModTime: time.Unix(200, 0)},
	}
	l := Build(Input{DataFS: m}).Log
	if l == nil || l.File != "main.LOG" || len(l.Lines) != LogLines {
		t.Fatalf("%+v", l)
	}
	if l.Lines[0] != "line 72" || l.Lines[LogLines-2] != "line 120" {
		t.Errorf("%q ... %q", l.Lines[0], l.Lines[LogLines-2])
	}
	if last := l.Lines[LogLines-1]; len(last) != MaxLineLen+3 || !strings.HasSuffix(last, "...") {
		t.Errorf("long line: %d", len(last))
	}
	if Build(Input{DataFS: fstest.MapFS{"a.json": {Data: []byte("{}")}}}).Log != nil {
		t.Error("no log file means no log section")
	}
	if Build(Input{}).Log != nil || len(Build(Input{}).Files) != 0 {
		t.Error("no folder")
	}
}

func TestHugeLogIsOnlyReadFromTheEnd(t *testing.T) {
	big := strings.Repeat("0123456789\n", 20000) + "the end\n" // ~220 KB
	l := Build(Input{DataFS: fstest.MapFS{"w.log": {Data: []byte(big)}}}).Log
	if l == nil || l.Lines[len(l.Lines)-1] != "the end" {
		t.Fatalf("%+v", l)
	}
}

func TestRenderIsStableValidJSONWithAFinalNewline(t *testing.T) {
	in := plantedInput()
	a, b := Render(in), Render(in)
	if a != b {
		t.Error("rendering twice differs")
	}
	if !strings.HasSuffix(a, "}\n") || !json.Valid([]byte(a)) || strings.HasPrefix(a, "\ufeff") {
		t.Errorf("not a clean JSON file")
	}
	if !strings.Contains(a, `"kind": "winforge-diagnostic-report"`) {
		t.Error("kind")
	}
	// no HTML escaping: <user> stays readable
	if strings.Contains(a, `\u003c`) {
		t.Error("escaped")
	}
}

func TestWingetMissingIsReported(t *testing.T) {
	r := Build(Input{})
	if r.Winget.Found || r.Winget.Version != "" || r.Log != nil || r.Scans == nil || r.Files == nil {
		t.Errorf("%+v", r)
	}
}
