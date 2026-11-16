// Package envdoctor checks a PC's environment variables the way a careful person would: it reads
// the user and machine PATH and the usual *_HOME variables, and reports dead, duplicated or odd
// entries, homes that point nowhere, and tools installed more than once.
//
// The scan only reads the registry, the environment and the file system. It never runs a program
// found on PATH. Fixes touch user-scope variables only (machine variables are reported, never
// changed), always save a backup of what they replace first, and can be undone with Restore.
//
// The logic here is pure; the registry, the disk and the broadcast go through System, which a
// Windows file implements and tests replace with an in-memory fake. The package writes no English:
// the page builds its sentences from a finding's kind, subject and paths.
package envdoctor

import (
	"strconv"
	"strings"
	"time"
)

// Scope says whose variable something is.
type Scope string

const (
	User    Scope = "user"
	Machine Scope = "machine"
)

// Registry value types that environment variables use (REG_SZ and REG_EXPAND_SZ).
const (
	TypeString uint32 = 1
	TypeExpand uint32 = 2
)

// Finding kinds. They are stable ids the page translates.
const (
	KindPathMissing    = "path-missing"
	KindPathNotDir     = "path-not-dir"
	KindPathDuplicate  = "path-duplicate"
	KindPathEmpty      = "path-empty"
	KindPathRelative   = "path-relative"
	KindPathLong       = "path-long"
	KindHomeMissing    = "home-missing"
	KindHomeInvalid    = "home-invalid"
	KindHomeMismatch   = "home-mismatch"
	KindToolShadowed   = "tool-shadowed"
	KindToolStoreAlias = "tool-store-alias"
)

// Severities: a problem likely breaks something, a note is housekeeping.
const (
	SeverityProblem = "problem"
	SeverityNote    = "note"
)

// What Fix does to a fixable finding.
const (
	FixRemovePathEntry = "remove-path-entry"
	FixUnsetVariable   = "unset-variable"
)

const (
	longPathChars       = 1800 // a PATH value longer than this is worth a note
	maxBackups          = 10
	maxExpandDepth      = 8
	storeAliasDirMarker = `\windowsapps\`
)

// PathKind is what a path is.
type PathKind int

const (
	Missing PathKind = iota // nothing there (or a broken link)
	File
	Dir
)

// Raw is one stored variable: its text exactly as in the registry (a REG_EXPAND_SZ value keeps its
// %VAR% references) and its registry type.
type Raw struct {
	Value string
	Type  uint32
}

// System is everything the package needs from the machine.
type System interface {
	// Env returns every string variable of the scope, keyed by its registry name, unexpanded.
	Env(scope Scope) (map[string]Raw, error)
	// SetUserVar and DeleteUserVar change HKCU\Environment. They never touch the machine scope.
	SetUserVar(name, value string, typ uint32) error
	DeleteUserVar(name string) error
	// Broadcast tells running programs that the environment changed. Best effort.
	Broadcast() error
	// Stat follows links. It never runs anything.
	Stat(path string) PathKind
	// Getenv reads the process environment: where USERPROFILE, SystemRoot and PATHEXT come from.
	Getenv(name string) string
	// BackupDir is the folder that holds the backups. It may not exist yet.
	BackupDir() (string, error)
}

// Finding is one thing the scan noticed. Subject is the variable name, the PATH entry as written,
// or the tool name; Paths are the folders or files it is about (for tool-shadowed, every copy,
// the winner first). Fixable findings carry the kind of change Fix would make.
type Finding struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Scope    Scope    `json:"scope"`
	Subject  string   `json:"subject"`
	Paths    []string `json:"paths"`
	Fixable  bool     `json:"fixable"`
	FixKind  string   `json:"fixKind"`

	variable string // registry name of the variable a fix changes
	index    int    // for a PATH entry: its position among the segments
}

// Tool is a program name and every copy of it found on PATH, the one that runs first at the top.
type Tool struct {
	Name  string   `json:"name"`
	Found []string `json:"found"`
}

// Var is one of the *_HOME style variables that is set, as stored.
type Var struct {
	Name  string `json:"name"`
	Scope Scope  `json:"scope"`
	Value string `json:"value"`
}

// Report is what a scan found. No slice is ever nil.
type Report struct {
	Findings    []Finding `json:"findings"`
	UserPath    []string  `json:"userPath"`
	MachinePath []string  `json:"machinePath"`
	Tools       []Tool    `json:"tools"`
	Vars        []Var     `json:"vars"`
}

// Manager scans and fixes. Now may be nil (the real clock).
type Manager struct {
	Sys System
	Now func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// scanState is a report plus what a fix needs to act on it.
type scanState struct {
	report   Report
	userEnv  map[string]Raw
	pathName string // registry name of the user PATH variable
}

// lookupFold finds a variable by name ignoring case, as Windows does. It returns the stored name.
func lookupFold(env map[string]Raw, name string) (string, Raw, bool) {
	if r, ok := env[name]; ok {
		return name, r, true
	}
	best, found := "", false
	for k := range env {
		if strings.EqualFold(k, name) && (!found || k < best) {
			best, found = k, true
		}
	}
	if !found {
		return "", Raw{}, false
	}
	return best, env[best], true
}

// resolver expands %VAR% the way Windows builds the environment: user variables win over machine
// ones, and the process environment supplies the rest (USERPROFILE, SystemRoot...).
type resolver struct {
	sys           System
	user, machine map[string]Raw
}

func (r *resolver) lookup(name string, depth int) (string, bool) {
	if depth > maxExpandDepth {
		return "", false
	}
	for _, env := range []map[string]Raw{r.user, r.machine} {
		if _, raw, ok := lookupFold(env, name); ok {
			if raw.Type == TypeExpand {
				return expand(raw.Value, func(n string) (string, bool) { return r.lookup(n, depth+1) }), true
			}
			return raw.Value, true
		}
	}
	if v := r.sys.Getenv(name); v != "" {
		return v, true
	}
	return "", false
}

func (r *resolver) expand(s string) string {
	return expand(s, func(n string) (string, bool) { return r.lookup(n, 0) })
}

// Scan reads the environment and reports what looks wrong. It writes nothing.
func (m *Manager) Scan() (Report, error) {
	st, err := m.scan()
	if err != nil {
		return Report{}, err
	}
	return st.report, nil
}

func (m *Manager) scan() (*scanState, error) {
	userEnv, err := m.Sys.Env(User)
	if err != nil {
		return nil, err
	}
	machineEnv, err := m.Sys.Env(Machine)
	if err != nil {
		return nil, err
	}
	res := &resolver{sys: m.Sys, user: userEnv, machine: machineEnv}

	pathName, userRaw, _ := lookupFold(userEnv, "Path")
	if pathName == "" {
		pathName = "Path"
	}
	_, machineRaw, _ := lookupFold(machineEnv, "Path")

	machine := m.analyzePath(res, Machine, "", splitPath(machineRaw.Value), machineRaw.Value, nil)
	user := m.analyzePath(res, User, pathName, splitPath(userRaw.Value), userRaw.Value, machine.firstSeen)

	tools := m.findTools(res, effectiveDirs(machine.dirs, user.dirs))
	homes := m.checkHomes(res, tools)

	// The person's own PATH first, then the machine's: only the first can be fixed.
	var all []Finding
	all = append(all, user.findings...)
	all = append(all, machine.findings...)
	all = append(all, homes.findings...)
	all = append(all, toolFindings(tools)...)

	rep := Report{
		Findings:    assignIDs(all),
		UserPath:    nonNil(splitPath(userRaw.Value)),
		MachinePath: nonNil(splitPath(machineRaw.Value)),
		Tools:       make([]Tool, 0, len(tools)),
		Vars:        nonNilVars(homes.vars),
	}
	for _, t := range tools {
		paths := make([]string, 0, len(t.found))
		for _, f := range t.found {
			paths = append(paths, f.path)
		}
		rep.Tools = append(rep.Tools, Tool{Name: t.name, Found: paths})
	}
	return &scanState{report: rep, userEnv: userEnv, pathName: pathName}, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilVars(v []Var) []Var {
	if v == nil {
		return []Var{}
	}
	return v
}

// assignIDs gives findings ids that are stable inside one scan: kind|scope|subject, with a #n
// suffix for the second and later findings that would otherwise share one (the same entry written
// twice, or two empty segments).
func assignIDs(in []Finding) []Finding {
	seen := map[string]int{}
	out := make([]Finding, len(in))
	for i, f := range in {
		base := f.Kind + "|" + string(f.Scope) + "|" + f.Subject
		seen[base]++
		f.ID = base
		if n := seen[base]; n > 1 {
			f.ID = base + "#" + strconv.Itoa(n)
		}
		out[i] = f
	}
	return out
}

// pathResult is what looking at one PATH variable produced.
type pathResult struct {
	findings  []Finding
	dirs      []dirEntry        // usable directories, in order
	firstSeen map[string]string // dirKey -> normalised directory of its first entry
}

type dirEntry struct {
	dir   string
	scope Scope
}

func effectiveDirs(machine, user []dirEntry) []dirEntry {
	var out []dirEntry
	seen := map[string]bool{}
	for _, d := range append(append([]dirEntry{}, machine...), user...) {
		k := dirKey(d.dir)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	return out
}

// analyzePath looks at every segment of one PATH. others holds the machine's directories when the
// user's PATH is being looked at: a user entry that repeats one of them is redundant.
func (m *Manager) analyzePath(res *resolver, scope Scope, varName string, segs []string, rawValue string, others map[string]string) pathResult {
	out := pathResult{firstSeen: map[string]string{}}
	fixable := scope == User
	add := func(kind, sev, subject string, paths []string, idx int, canFix bool) {
		f := Finding{Kind: kind, Severity: sev, Scope: scope, Subject: subject, Paths: nonNil(paths), variable: varName, index: idx}
		if fixable && canFix {
			f.Fixable, f.FixKind = true, FixRemovePathEntry
		}
		out.findings = append(out.findings, f)
	}
	if n := len([]rune(rawValue)); n > longPathChars {
		add(KindPathLong, SeverityNote, "PATH", nil, -1, false)
	}
	for i, seg := range segs {
		clean := cleanSegment(seg)
		if clean == "" {
			add(KindPathEmpty, SeverityNote, seg, nil, i, true)
			continue
		}
		expanded := res.expand(clean)
		if unresolved(expanded) {
			add(KindPathMissing, SeverityNote, seg, []string{expanded}, i, true)
			continue
		}
		if !isAbs(strings.ReplaceAll(expanded, "/", `\`)) {
			add(KindPathRelative, SeverityProblem, seg, []string{expanded}, i, false)
			continue
		}
		dir := normDir(expanded)
		key := dirKey(dir)
		kind := m.Sys.Stat(dir)
		switch kind {
		case Missing:
			add(KindPathMissing, SeverityNote, seg, []string{dir}, i, true)
		case File:
			add(KindPathNotDir, SeverityNote, seg, []string{dir}, i, false)
		}
		if kind == Dir {
			out.dirs = append(out.dirs, dirEntry{dir: dir, scope: scope})
		}
		if first, dup := out.firstSeen[key]; dup {
			if kind != Missing { // a dead entry is already reported as dead, and goes away with it
				add(KindPathDuplicate, SeverityNote, seg, []string{first}, i, true)
			}
			continue
		}
		out.firstSeen[key] = dir
		if other, dup := others[key]; dup && kind != Missing {
			add(KindPathDuplicate, SeverityNote, seg, []string{other}, i, true)
		}
	}
	return out
}
