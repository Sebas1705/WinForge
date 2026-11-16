package envdoctor

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEmptyMachineReportsNothingAndNoNilSlices(t *testing.T) {
	f := newFake(t)
	r := scan(t, f)
	if len(r.Findings) != 0 || len(r.UserPath) != 0 || len(r.MachinePath) != 0 || len(r.Vars) != 0 {
		t.Fatalf("%+v", r)
	}
	if r.Findings == nil || r.UserPath == nil || r.MachinePath == nil || r.Vars == nil || r.Tools == nil {
		t.Fatalf("slices must not be nil: %+v", r)
	}
	for _, tl := range r.Tools {
		if tl.Found == nil || len(tl.Found) != 0 {
			t.Errorf("%s: found = %#v", tl.Name, tl.Found)
		}
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "null") {
		t.Errorf("JSON has null: %s", b)
	}
}

func TestFindingJSONShape(t *testing.T) {
	f := newFake(t)
	f.setUser("Path", `C:\gone`)
	r := scan(t, f)
	b, _ := json.Marshal(r.Findings[0])
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "kind", "severity", "scope", "subject", "paths", "fixable", "fixKind"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing json key %q in %s", k, b)
		}
	}
	if len(m) != 8 {
		t.Errorf("unexpected keys: %s", b)
	}
}

func TestPathEntryKinds(t *testing.T) {
	f := newFake(t)
	f.dir(`C:\ok`, `C:\Program Files\App`).file(`C:\afile`)
	f.setUser("Path", strings.Join([]string{
		`C:\ok`,
		`C:\gone`,
		`C:\afile`,
		``,
		`bin`,
		`.\tools`,
		`C:foo`,
		`"C:\Program Files\App"`,
		`%USERPROFILE%\nowhere`,
		`%UNKNOWN%\bin`,
	}, ";"))
	r := scan(t, f)

	g := mustFind(t, r, `path-missing|user|C:\gone`)
	if g.Severity != SeverityNote || g.Scope != User || !g.Fixable || g.FixKind != FixRemovePathEntry || !reflect.DeepEqual(g.Paths, []string{`C:\gone`}) {
		t.Errorf("%+v", g)
	}
	nf := mustFind(t, r, `path-not-dir|user|C:\afile`)
	if nf.Fixable || nf.FixKind != "" {
		t.Errorf("a file on PATH is reported, not fixed: %+v", nf)
	}
	e := mustFind(t, r, `path-empty|user|`)
	if !e.Fixable || len(e.Paths) != 0 || e.Paths == nil {
		t.Errorf("%+v", e)
	}
	for _, rel := range []string{`bin`, `.\tools`, `C:foo`} {
		fr := mustFind(t, r, `path-relative|user|`+rel)
		if fr.Fixable || fr.Severity != SeverityProblem {
			t.Errorf("relative is a problem and not fixable: %+v", fr)
		}
	}
	m := mustFind(t, r, `path-missing|user|%USERPROFILE%\nowhere`)
	if !reflect.DeepEqual(m.Paths, []string{`C:\Users\me\nowhere`}) {
		t.Errorf("paths show the expanded folder: %+v", m.Paths)
	}
	u := mustFind(t, r, `path-missing|user|%UNKNOWN%\bin`)
	if !u.Fixable || !reflect.DeepEqual(u.Paths, []string{`%UNKNOWN%\bin`}) {
		t.Errorf("an unknown variable never resolves to a folder: %+v", u)
	}
	noFinding(t, r, `path-missing|user|C:\ok`)
	noFinding(t, r, `path-missing|user|"C:\Program Files\App"`)
	if len(r.UserPath) != 10 || r.UserPath[7] != `"C:\Program Files\App"` {
		t.Errorf("userPath keeps the raw segments: %q", r.UserPath)
	}
}

func TestEmptySegments(t *testing.T) {
	cases := map[string]int{
		`C:\a;`:        1, // trailing separator
		`;C:\a`:        1,
		`C:\a;;C:\b`:   1,
		`;;`:           3,
		`C:\a; ;C:\b`:  1, // blank is empty
		`C:\a`:         0,
		``:             0,
		`C:\a;"";C:\b`: 1,
	}
	for v, want := range cases {
		f := newFake(t).dir(`C:\a`, `C:\b`)
		f.setUser("Path", v)
		r := scan(t, f)
		if got := countKind(r, KindPathEmpty); got != want {
			t.Errorf("%q: %d empty findings, want %d (%v)", v, got, want, ids(r))
		}
	}
}

func TestFindingIDsAreUniqueEvenForRepeatedSegments(t *testing.T) {
	f := newFake(t)
	f.setUser("Path", `;;C:\gone;C:\gone;C:\gone`)
	r := scan(t, f)
	seen := map[string]bool{}
	for _, fd := range r.Findings {
		if seen[fd.ID] {
			t.Fatalf("duplicate id %q in %v", fd.ID, ids(r))
		}
		seen[fd.ID] = true
	}
	if countKind(r, KindPathEmpty) != 2 || countKind(r, KindPathMissing) != 3 {
		t.Errorf("%v", ids(r))
	}
	mustFind(t, r, `path-missing|user|C:\gone#3`)
}

func TestDuplicates(t *testing.T) {
	f := newFake(t).dir(`C:\Tools`, `C:\Users\me\bin`, `C:\Program Files\X`, `C:\Windows`)
	f.setUser("Path", strings.Join([]string{
		`C:\Tools`,
		`c:\tools\`, // case and trailing slash
		`C:/Tools`,  // forward slash
		`%USERPROFILE%\bin`,
		`C:\Users\me\bin\`, // expansion normalised
		`"C:\Program Files\X"`,
		`C:\Program Files\X`, // quotes
		`C:\Tools\sub\..`,    // dot dot
	}, ";"))
	r := scan(t, f)
	dups := []string{}
	for _, fd := range r.Findings {
		if fd.Kind == KindPathDuplicate {
			dups = append(dups, fd.Subject)
			if !fd.Fixable || fd.FixKind != FixRemovePathEntry || fd.Severity != SeverityNote {
				t.Errorf("%+v", fd)
			}
		}
	}
	want := []string{`c:\tools\`, `C:/Tools`, `C:\Users\me\bin\`, `C:\Program Files\X`, `C:\Tools\sub\..`}
	if !reflect.DeepEqual(dups, want) {
		t.Errorf("duplicates = %q, want %q", dups, want)
	}
	d := mustFind(t, r, `path-duplicate|user|c:\tools\`)
	if !reflect.DeepEqual(d.Paths, []string{`C:\Tools`}) {
		t.Errorf("paths name the entry that stays: %+v", d.Paths)
	}
	// The first of each pair is never reported.
	noFinding(t, r, `path-duplicate|user|C:\Tools`)
}

func TestUserEntryRepeatingMachineEntry(t *testing.T) {
	f := newFake(t).dir(`C:\Windows`, `C:\Tools`, `C:\Mine`)
	f.setMachineX("Path", `%SystemRoot%;C:\Tools;C:\Tools`)
	f.setUser("Path", `C:\mine;c:\windows\;C:\TOOLS`)
	r := scan(t, f)

	ud := mustFind(t, r, `path-duplicate|user|c:\windows\`)
	if !ud.Fixable || ud.Scope != User || !reflect.DeepEqual(ud.Paths, []string{`C:\Windows`}) {
		t.Errorf("%+v", ud)
	}
	mustFind(t, r, `path-duplicate|user|C:\TOOLS`)
	md := mustFind(t, r, `path-duplicate|machine|C:\Tools`)
	if md.Fixable || md.FixKind != "" || md.Scope != Machine {
		t.Errorf("machine duplicates are reported, never offered: %+v", md)
	}
	noFinding(t, r, `path-duplicate|user|C:\mine`)
	for _, fd := range r.Findings {
		if fd.Scope == Machine && fd.Fixable {
			t.Errorf("machine findings are never fixable: %+v", fd)
		}
	}
}

func TestADeadDuplicateIsOnlyDead(t *testing.T) {
	f := newFake(t)
	f.setUser("Path", `C:\gone;C:\gone`)
	r := scan(t, f)
	if countKind(r, KindPathDuplicate) != 0 || countKind(r, KindPathMissing) != 2 {
		t.Errorf("%v", ids(r))
	}
}

func TestSymlinkedFolderCounts(t *testing.T) {
	f := newFake(t).dir(`C:\real`).link(`C:\link`, `C:\real`)
	f.link(`C:\broken`, `C:\nowhere`)
	f.setUser("Path", `C:\link;C:\broken`)
	r := scan(t, f)
	noFinding(t, r, `path-missing|user|C:\link`)
	mustFind(t, r, `path-missing|user|C:\broken`)
}

func TestLongPath(t *testing.T) {
	f := newFake(t)
	var parts []string
	for len(strings.Join(parts, ";")) <= 2000 {
		parts = append(parts, `C:\some\fairly\long\folder\name\`+strings.Repeat("x", 20))
		f.dir(parts[len(parts)-1])
	}
	f.setUser("Path", strings.Join(parts, ";"))
	r := scan(t, f)
	l := mustFind(t, r, `path-long|user|PATH`)
	if l.Severity != SeverityNote || l.Fixable || len(l.Paths) != 0 {
		t.Errorf("%+v", l)
	}
	// Exactly at the limit is fine, one over is not.
	for n, want := range map[int]int{1800: 0, 1801: 1} {
		g := newFake(t)
		g.setUser("Path", strings.Repeat("a", n))
		if got := countKind(scan(t, g), KindPathLong); got != want {
			t.Errorf("%d chars: %d long findings, want %d", n, got, want)
		}
	}
	// Machine too, and counted in characters, not bytes.
	g := newFake(t)
	g.setMachine("Path", strings.Repeat("é", 1500))
	if countKind(scan(t, g), KindPathLong) != 0 {
		t.Error("1500 two-byte characters are short")
	}
	g.setMachine("Path", strings.Repeat("é", 1900))
	r2 := scan(t, g)
	if f2 := mustFind(t, r2, `path-long|machine|PATH`); f2.Fixable {
		t.Error("machine PATH is never changed")
	}
}

func TestPathVariableNameCaseDoesNotMatter(t *testing.T) {
	f := newFake(t).dir(`C:\a`)
	f.user["PATH"] = Raw{Value: `C:\a;C:\gone`, Type: TypeExpand}
	f.machine["path"] = Raw{Value: `C:\a`, Type: TypeString}
	r := scan(t, f)
	mustFind(t, r, `path-missing|user|C:\gone`)
	mustFind(t, r, `path-duplicate|user|C:\a`)
	if len(r.UserPath) != 2 || len(r.MachinePath) != 1 {
		t.Errorf("%+v", r)
	}
}

func TestExpansionUsesUserThenMachineThenProcess(t *testing.T) {
	f := newFake(t).dir(`C:\fromuser\bin`, `C:\frommachine\bin`, `D:\nested\bin`)
	f.setUser("Foo", `C:\fromuser`)
	f.setMachine("Foo", `C:\ignored`)
	f.setMachine("Bar", `C:\frommachine`)
	f.setMachineX("Nested", `D:\%LAYER%`)
	f.setMachine("Layer", `nested`)
	f.setUser("Path", `%FOO%\bin;%bar%\bin;%NESTED%\bin`)
	r := scan(t, f)
	if len(r.Findings) != 0 {
		t.Errorf("all three resolve: %v", ids(r))
	}
}

func TestExpansionLoopDoesNotHang(t *testing.T) {
	f := newFake(t)
	f.setUserX("A", `%B%`)
	f.setUserX("B", `%A%`)
	f.setUser("Path", `%A%\bin`)
	r := scan(t, f)
	mustFind(t, r, `path-missing|user|%A%\bin`)
}

func TestHomeMissing(t *testing.T) {
	f := newFake(t)
	f.setUser("JAVA_HOME", `C:\jdk-gone`)
	f.setMachine("GOROOT", `C:\Go`)
	f.setUserX("ANDROID_HOME", `%LOCALAPPDATA%\Android\Sdk`)
	f.file(`C:\afile`)
	f.setUser("PYTHONHOME", `C:\afile`)
	f.setUser("M2_HOME", `maven`)
	r := scan(t, f)
	j := mustFind(t, r, `home-missing|user|JAVA_HOME`)
	if !j.Fixable || j.FixKind != FixUnsetVariable || j.Severity != SeverityProblem || !reflect.DeepEqual(j.Paths, []string{`C:\jdk-gone`}) {
		t.Errorf("%+v", j)
	}
	g := mustFind(t, r, `home-missing|machine|GOROOT`)
	if g.Fixable || g.FixKind != "" {
		t.Errorf("machine variables are never changed: %+v", g)
	}
	mustFind(t, r, `home-missing|user|ANDROID_HOME`) // %LOCALAPPDATA% is not even set here
	mustFind(t, r, `home-missing|user|PYTHONHOME`)   // a file is not a folder
	mustFind(t, r, `home-missing|user|M2_HOME`)      // relative
	if len(r.Vars) != 5 {
		t.Errorf("vars: %+v", r.Vars)
	}
}

func TestHomeInvalidMarkers(t *testing.T) {
	cases := []struct {
		name    string
		bad     []string
		goodSet [][]string // each entry makes the home valid
		dirs    bool
	}{
		{"JAVA_HOME", nil, [][]string{{`bin\java.exe`}}, false},
		{"GOROOT", nil, [][]string{{`bin\go.exe`}}, false},
		{"DOTNET_ROOT", nil, [][]string{{`dotnet.exe`}}, false},
		{"GRADLE_HOME", nil, [][]string{{`bin\gradle.bat`}}, false},
		{"MAVEN_HOME", nil, [][]string{{`bin\mvn.cmd`}}, false},
		{"M2_HOME", nil, [][]string{{`bin\mvn.cmd`}}, false},
		{"PYTHONHOME", nil, [][]string{{`python.exe`}}, false},
		{"ANDROID_HOME", nil, [][]string{{`platform-tools`}, {`cmdline-tools`}}, true},
		{"ANDROID_SDK_ROOT", nil, [][]string{{`platform-tools`}, {`cmdline-tools`}}, true},
		{"CARGO_HOME", nil, [][]string{{`bin`}}, true},
		{"RUSTUP_HOME", nil, [][]string{{`toolchains`}, {`settings.toml`}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := `C:\homes\` + strings.ToLower(c.name)
			f := newFake(t).dir(home)
			f.setUser(c.name, home)
			r := scan(t, f)
			inv := mustFind(t, r, `home-invalid|user|`+c.name)
			if inv.Fixable || inv.Severity != SeverityProblem || !reflect.DeepEqual(inv.Paths, []string{home}) {
				t.Errorf("%+v", inv)
			}
			for _, good := range c.goodSet {
				g := newFake(t).dir(home)
				for _, rel := range good {
					p := home + `\` + rel
					if c.dirs || rel == "toolchains" {
						g.dir(p)
					} else {
						g.file(p)
					}
				}
				g.setUser(c.name, home+`\`) // trailing slash is fine
				if got := scan(t, g); len(got.Findings) != 0 {
					t.Errorf("valid with %v: %v", good, ids(got))
				}
			}
		})
	}
}

func TestHomeMarkerKindMatters(t *testing.T) {
	// A folder called java.exe is not java.
	f := newFake(t).dir(`C:\jdk`, `C:\jdk\bin\java.exe`)
	f.setUser("JAVA_HOME", `C:\jdk`)
	mustFind(t, scan(t, f), `home-invalid|user|JAVA_HOME`)
}

func TestHomeMismatch(t *testing.T) {
	f := newFake(t).dir(`C:\jdk17\bin`, `C:\jdk21\bin`, `C:\Go\bin`, `C:\other\bin`)
	f.file(`C:\jdk17\bin\java.exe`, `C:\jdk21\bin\java.exe`, `C:\Go\bin\go.exe`, `C:\other\bin\go.exe`)
	f.setMachine("Path", `C:\jdk21\bin;C:\jdk17\bin;C:\other\bin;C:\Go\bin`)
	f.setUser("JAVA_HOME", `C:\jdk17`)
	f.setUser("GOROOT", `C:\Go`)
	r := scan(t, f)
	jm := mustFind(t, r, `home-mismatch|user|JAVA_HOME`)
	if jm.Fixable || jm.Severity != SeverityProblem || !reflect.DeepEqual(jm.Paths, []string{`C:\jdk17\bin\java.exe`, `C:\jdk21\bin\java.exe`}) {
		t.Errorf("%+v", jm)
	}
	mustFind(t, r, `home-mismatch|user|GOROOT`)
	mustFind(t, r, `tool-shadowed|machine|java`)

	// Same winner: no mismatch, however it is spelled.
	g := newFake(t).dir(`C:\jdk17\bin`).file(`C:\jdk17\bin\java.exe`)
	g.setMachine("Path", `c:\JDK17\bin\`)
	g.setUser("JAVA_HOME", `"C:\jdk17"`)
	if got := scan(t, g); len(got.Findings) != 0 {
		t.Errorf("%v", ids(got))
	}
	// Home is fine but java is not on PATH at all: nothing to compare.
	h := newFake(t).dir(`C:\jdk17\bin`).file(`C:\jdk17\bin\java.exe`)
	h.setUser("JAVA_HOME", `C:\jdk17`)
	if got := scan(t, h); len(got.Findings) != 0 {
		t.Errorf("%v", ids(got))
	}
}

func TestMismatchOnlyForTheVariableInEffect(t *testing.T) {
	f := newFake(t).dir(`C:\jdk17\bin`, `C:\jdk21\bin`)
	f.file(`C:\jdk17\bin\java.exe`, `C:\jdk21\bin\java.exe`)
	f.setMachine("Path", `C:\jdk21\bin`)
	f.setMachine("JAVA_HOME", `C:\jdk17`)
	f.setUser("JAVA_HOME", `C:\jdk21`)
	r := scan(t, f)
	noFinding(t, r, `home-mismatch|machine|JAVA_HOME`)
	noFinding(t, r, `home-mismatch|user|JAVA_HOME`)
	if len(r.Vars) != 2 || r.Vars[0].Scope != User || r.Vars[1].Scope != Machine {
		t.Errorf("%+v", r.Vars)
	}
	// Machine home alone with a different java winning: mismatch, in machine scope, not fixable.
	g := newFake(t).dir(`C:\jdk17\bin`, `C:\jdk21\bin`)
	g.file(`C:\jdk17\bin\java.exe`, `C:\jdk21\bin\java.exe`)
	g.setMachine("Path", `C:\jdk21\bin`)
	g.setMachine("JAVA_HOME", `C:\jdk17`)
	mm := mustFind(t, scan(t, g), `home-mismatch|machine|JAVA_HOME`)
	if mm.Fixable {
		t.Error("machine mismatch is not fixable")
	}
}

func TestBothScopesCheckedForMissing(t *testing.T) {
	f := newFake(t)
	f.setUser("JAVA_HOME", `C:\a`)
	f.setMachine("JAVA_HOME", `C:\b`)
	r := scan(t, f)
	if !mustFind(t, r, `home-missing|user|JAVA_HOME`).Fixable || mustFind(t, r, `home-missing|machine|JAVA_HOME`).Fixable {
		t.Error("only the user's is fixable")
	}
}

func TestEmptyHomeValueIsListedNotJudged(t *testing.T) {
	f := newFake(t)
	f.setUser("JAVA_HOME", ``)
	f.setUser("java_home", `  `) // both spellings are the same variable; the first by name wins listing
	r := scan(t, f)
	if len(r.Findings) != 0 {
		t.Errorf("%v", ids(r))
	}
}

func TestToolShadowingAndOrder(t *testing.T) {
	f := newFake(t)
	f.dir(`C:\m1`, `C:\m2`, `C:\u1`)
	f.file(`C:\m1\node.exe`, `C:\u1\node.exe`, `C:\m2\git.exe`, `C:\u1\npm.cmd`, `C:\m2\npm.cmd`)
	f.setMachine("Path", `C:\m1;C:\m2`)
	f.setUser("Path", `C:\u1`)
	r := scan(t, f)

	n := mustFind(t, r, `tool-shadowed|machine|node`)
	if !reflect.DeepEqual(n.Paths, []string{`C:\m1\node.exe`, `C:\u1\node.exe`}) || n.Fixable || n.Severity != SeverityNote {
		t.Errorf("machine PATH comes first, winner first in the list: %+v", n)
	}
	// npm: machine copy wins even though the user's dir appears too.
	np := mustFind(t, r, `tool-shadowed|machine|npm`)
	if np.Paths[0] != `C:\m2\npm.cmd` {
		t.Errorf("%+v", np)
	}
	noFinding(t, r, `tool-shadowed|machine|git`)

	var tools = map[string][]string{}
	for _, tl := range r.Tools {
		tools[tl.Name] = tl.Found
	}
	if !reflect.DeepEqual(tools["git"], []string{`C:\m2\git.exe`}) || len(tools["java"]) != 0 {
		t.Errorf("%v", tools)
	}
	if len(r.Tools) != len(toolNames) {
		t.Errorf("every tool is listed even when absent: %d", len(r.Tools))
	}
}

func TestShadowedWinnerScopeIsTheWinnersPath(t *testing.T) {
	f := newFake(t).dir(`C:\m`, `C:\u`)
	f.file(`C:\m\go.exe`, `C:\u\go.exe`)
	f.setMachine("Path", `C:\m`)
	f.setUser("Path", `C:\u`)
	mustFind(t, scan(t, f), `tool-shadowed|machine|go`)
	g := newFake(t).dir(`C:\m`, `C:\u`)
	g.file(`C:\u\go.exe`)
	g.file(`C:\m\other.exe`)
	g.setMachine("Path", `C:\m`)
	g.setUser("Path", `C:\u`)
	if r := scan(t, g); countKind(r, KindToolShadowed) != 0 {
		t.Errorf("%v", ids(r))
	}
}

func TestSameFolderTwiceIsNotShadowing(t *testing.T) {
	f := newFake(t).dir(`C:\a`).file(`C:\a\node.exe`)
	f.setMachine("Path", `C:\a`)
	f.setUser("Path", `C:\a\;c:\A`)
	if r := scan(t, f); countKind(r, KindToolShadowed) != 0 {
		t.Errorf("%v", ids(r))
	}
}

func TestPathExtOrderAndDefault(t *testing.T) {
	// With PATHEXT unset the Windows default applies; a .ps1 or no extension is never a match.
	f := newFake(t).dir(`C:\a`, `C:\b`)
	f.file(`C:\a\gradle.ps1`, `C:\a\gradle`, `C:\b\gradle.bat`)
	f.setMachine("Path", `C:\a;C:\b`)
	r := scan(t, f)
	for _, tl := range r.Tools {
		if tl.Name == "gradle" && !reflect.DeepEqual(tl.Found, []string{`C:\b\gradle.bat`}) {
			t.Errorf("%v", tl.Found)
		}
	}
	// PATHEXT decides which extension counts, and the first extension in a folder wins.
	g := newFake(t).dir(`C:\a`)
	g.env["PATHEXT"] = ".CMD;.EXE"
	g.file(`C:\a\mvn.cmd`, `C:\a\mvn.exe`)
	g.setMachine("Path", `C:\a`)
	for _, tl := range scan(t, g).Tools {
		if tl.Name == "mvn" && !reflect.DeepEqual(tl.Found, []string{`C:\a\mvn.cmd`}) {
			t.Errorf("%v", tl.Found)
		}
	}
	if got := pathExts(" exe ; .Bat;;"); !reflect.DeepEqual(got, []string{".exe", ".bat"}) {
		t.Errorf("%v", got)
	}
}

func TestStoreAlias(t *testing.T) {
	alias := `C:\Users\me\AppData\Local\Microsoft\WindowsApps`
	f := newFake(t).dir(alias, `C:\Python312`)
	f.file(alias+`\python.exe`, alias+`\python3.exe`, `C:\Python312\python.exe`, alias+`\git.exe`)
	f.setUserX("Path", `%USERPROFILE%\AppData\Local\Microsoft\WindowsApps;C:\Python312`)
	r := scan(t, f)
	a := mustFind(t, r, `tool-store-alias|user|python`)
	if a.Fixable || a.Severity != SeverityProblem || !reflect.DeepEqual(a.Paths, []string{alias + `\python.exe`}) {
		t.Errorf("%+v", a)
	}
	mustFind(t, r, `tool-store-alias|user|python3`)
	mustFind(t, r, `tool-shadowed|user|python`)
	noFinding(t, r, `tool-store-alias|user|git`) // only python names are Store stubs worth flagging
	if countKind(r, KindToolStoreAlias) != 2 {
		t.Errorf("%v", ids(r))
	}

	// The real python wins: no alias finding, only the shadowing note.
	g := newFake(t).dir(alias, `C:\Python312`)
	g.file(alias+`\python.exe`, `C:\Python312\python.exe`)
	g.setUser("Path", `C:\Python312;`+alias)
	r2 := scan(t, g)
	noFinding(t, r2, `tool-store-alias|user|python`)
	mustFind(t, r2, `tool-shadowed|user|python`)
}

func TestIDsAreUniqueAcrossAWholeMessyReport(t *testing.T) {
	f := newFake(t).dir(`C:\a`)
	f.setMachine("Path", `C:\a;C:\a;;C:\x`)
	f.setUser("Path", `C:\a;C:\x;;C:\x;;rel`)
	f.setUser("JAVA_HOME", `C:\none`)
	r := scan(t, f)
	seen := map[string]bool{}
	for _, fd := range r.Findings {
		if seen[fd.ID] {
			t.Errorf("duplicate id %q", fd.ID)
		}
		seen[fd.ID] = true
		if fd.Paths == nil {
			t.Errorf("paths is never nil: %+v", fd)
		}
		if fd.Fixable && (fd.Scope != User || fd.FixKind == "") {
			t.Errorf("%+v", fd)
		}
		if !fd.Fixable && fd.FixKind != "" {
			t.Errorf("%+v", fd)
		}
	}
}

func TestScanWritesNothing(t *testing.T) {
	f := newFake(t)
	f.setUser("Path", `C:\gone;;C:\gone`)
	f.setUser("JAVA_HOME", `C:\none`)
	scan(t, f)
	if f.writeCount() != 0 || f.broadcasts != 0 {
		t.Errorf("a scan is read-only: %v", f.writes)
	}
}

func TestScanTreatsEveryEntryAsDataOnly(t *testing.T) {
	// Entries that look like commands or options are just odd folders.
	f := newFake(t)
	f.setUser("Path", `C:\x & calc;"C:\y" --flag;$(whoami);%COMSPEC%`)
	r := scan(t, f)
	if len(r.UserPath) != 4 {
		t.Fatalf("%q", r.UserPath)
	}
	for _, fd := range r.Findings {
		if fd.Kind != KindPathMissing && fd.Kind != KindPathRelative {
			t.Errorf("%+v", fd)
		}
	}
}
