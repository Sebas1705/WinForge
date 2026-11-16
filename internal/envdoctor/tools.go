package envdoctor

import "strings"

// toolNames are the programs whose copies on PATH are listed. python3 is only there to spot the
// Microsoft Store stub, which answers to both names.
var toolNames = []string{"java", "javac", "python", "python3", "pip", "node", "npm", "git", "go", "dotnet", "gradle", "mvn", "cargo", "rustc", "adb"}

// storeAliasTools are the names the Microsoft Store "App execution aliases" claim.
var storeAliasTools = map[string]bool{"python": true, "python3": true}

type foundTool struct {
	path  string
	scope Scope // whose PATH supplied the directory
}

type toolScan struct {
	name  string
	found []foundTool // in PATH order: the first one wins
}

// pathExts reads PATHEXT, lower-cased, with Windows' own default when it is missing.
func pathExts(v string) []string {
	var out []string
	for _, e := range strings.Split(v, ";") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	return out
}

// findTools looks for every tool in every directory of the effective PATH (machine first, then
// the user's, like Windows). It only checks that a file exists: nothing is ever run.
func (m *Manager) findTools(res *resolver, dirs []dirEntry) []toolScan {
	exts := pathExts(m.Sys.Getenv("PATHEXT"))
	out := make([]toolScan, 0, len(toolNames))
	for _, name := range toolNames {
		t := toolScan{name: name, found: []foundTool{}}
		for _, d := range dirs {
			for _, ext := range exts {
				p := join(d.dir, name+ext)
				if m.Sys.Stat(p) == File {
					t.found = append(t.found, foundTool{path: p, scope: d.scope})
					break
				}
			}
		}
		out = append(out, t)
	}
	return out
}

func toolFindings(tools []toolScan) []Finding {
	var out []Finding
	for _, t := range tools {
		if len(t.found) == 0 {
			continue
		}
		winner := t.found[0]
		if len(t.found) > 1 {
			paths := make([]string, len(t.found))
			for i, f := range t.found {
				paths[i] = f.path
			}
			out = append(out, Finding{Kind: KindToolShadowed, Severity: SeverityNote, Scope: winner.scope, Subject: t.name, Paths: paths, index: -1})
		}
		if storeAliasTools[t.name] && strings.Contains(strings.ToLower(winner.path), storeAliasDirMarker) {
			out = append(out, Finding{Kind: KindToolStoreAlias, Severity: SeverityProblem, Scope: winner.scope, Subject: t.name, Paths: []string{winner.path}, index: -1})
		}
	}
	return out
}

// marker is something a real install of a tool has inside its home folder.
type marker struct {
	rel string
	dir bool
}

type homeSpec struct {
	name    string
	markers []marker // the folder is a real home when any of them is there
	tool    string   // the tool whose PATH winner should live in this home ("" for none)
	exe     string   // where that tool is inside the home
}

var homeSpecs = []homeSpec{
	{name: "JAVA_HOME", markers: []marker{{`bin\java.exe`, false}}, tool: "java", exe: `bin\java.exe`},
	{name: "ANDROID_HOME", markers: []marker{{"platform-tools", true}, {"cmdline-tools", true}}},
	{name: "ANDROID_SDK_ROOT", markers: []marker{{"platform-tools", true}, {"cmdline-tools", true}}},
	{name: "GOROOT", markers: []marker{{`bin\go.exe`, false}}, tool: "go", exe: `bin\go.exe`},
	{name: "DOTNET_ROOT", markers: []marker{{"dotnet.exe", false}}},
	{name: "GRADLE_HOME", markers: []marker{{`bin\gradle.bat`, false}}},
	{name: "MAVEN_HOME", markers: []marker{{`bin\mvn.cmd`, false}}},
	{name: "M2_HOME", markers: []marker{{`bin\mvn.cmd`, false}}},
	{name: "CARGO_HOME", markers: []marker{{"bin", true}}},
	{name: "RUSTUP_HOME", markers: []marker{{"toolchains", true}, {"settings.toml", false}}},
	{name: "PYTHONHOME", markers: []marker{{"python.exe", false}}},
}

type homeResult struct {
	findings []Finding
	vars     []Var
}

// checkHomes looks at every *_HOME style variable in both scopes. A home that is missing is
// reported in each scope that sets it; whether the PATH winner of java or go lives in the home is
// only checked for the variable that is actually in effect (the user's wins over the machine's).
func (m *Manager) checkHomes(res *resolver, tools []toolScan) homeResult {
	var out homeResult
	for _, spec := range homeSpecs {
		effective := Scope("")
		for _, sc := range []Scope{User, Machine} {
			env := res.user
			if sc == Machine {
				env = res.machine
			}
			name, raw, ok := lookupFold(env, spec.name)
			if !ok {
				continue
			}
			out.vars = append(out.vars, Var{Name: name, Scope: sc, Value: raw.Value})
			if effective == "" {
				effective = sc
			}
			val := cleanSegment(res.expand(raw.Value))
			if val == "" {
				continue
			}
			dir := normDir(val)
			add := func(kind, sev string, paths []string, fixable bool) {
				f := Finding{Kind: kind, Severity: sev, Scope: sc, Subject: name, Paths: paths, variable: name, index: -1}
				if fixable && sc == User {
					f.Fixable, f.FixKind = true, FixUnsetVariable
				}
				out.findings = append(out.findings, f)
			}
			if unresolved(dir) || !isAbs(dir) || m.Sys.Stat(dir) != Dir {
				add(KindHomeMissing, SeverityProblem, []string{dir}, true)
				continue
			}
			if !m.hasMarker(dir, spec.markers) {
				add(KindHomeInvalid, SeverityProblem, []string{dir}, false)
				continue
			}
			if spec.tool != "" && sc == effective {
				if winner, ok := firstOf(tools, spec.tool); ok {
					expected := normDir(join(dir, spec.exe))
					if dirKey(expected) != dirKey(normDir(winner.path)) {
						add(KindHomeMismatch, SeverityProblem, []string{expected, winner.path}, false)
					}
				}
			}
		}
	}
	return out
}

func (m *Manager) hasMarker(dir string, markers []marker) bool {
	for _, mk := range markers {
		want := File
		if mk.dir {
			want = Dir
		}
		if m.Sys.Stat(join(dir, mk.rel)) == want {
			return true
		}
	}
	return false
}

func firstOf(tools []toolScan, name string) (foundTool, bool) {
	for _, t := range tools {
		if t.name == name && len(t.found) > 0 {
			return t.found[0], true
		}
	}
	return foundTool{}, false
}
