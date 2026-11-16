package envdoctor

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSys is an in-memory machine: no registry, no disk (except the temp folder for backups).
type fakeSys struct {
	mu         sync.Mutex
	user       map[string]Raw
	machine    map[string]Raw
	env        map[string]string // the process environment
	fs         map[string]PathKind
	backupDir  string
	writes     []string // "set NAME" / "del NAME", in order
	broadcasts int
	failSet    map[string]error
	statCalls  int
}

func newFake(t *testing.T) *fakeSys {
	t.Helper()
	return &fakeSys{
		user:      map[string]Raw{},
		machine:   map[string]Raw{},
		env:       map[string]string{"USERPROFILE": `C:\Users\me`, "SystemRoot": `C:\Windows`},
		fs:        map[string]PathKind{},
		backupDir: t.TempDir(),
		failSet:   map[string]error{},
	}
}

func fsKey(p string) string { return dirKey(normDir(p)) }

func (f *fakeSys) dir(paths ...string) *fakeSys {
	for _, p := range paths {
		f.fs[fsKey(p)] = Dir
		f.parents(p)
	}
	return f
}

func (f *fakeSys) file(paths ...string) *fakeSys {
	for _, p := range paths {
		f.fs[fsKey(p)] = File
		f.parents(p)
	}
	return f
}

// parents makes every folder above p exist, as it does on a disk.
func (f *fakeSys) parents(p string) {
	n := normDir(p)
	for i := strings.LastIndex(n, `\`); i > 2; i = strings.LastIndex(n, `\`) {
		n = n[:i]
		if _, ok := f.fs[dirKey(n)]; !ok {
			f.fs[dirKey(n)] = Dir
		}
	}
}

// link makes from behave like what it points at (a symlink or junction that resolves).
func (f *fakeSys) link(from, to string) *fakeSys {
	f.fs[fsKey(from)] = f.fs[fsKey(to)]
	return f
}

func (f *fakeSys) setUser(name, val string)    { f.user[name] = Raw{Value: val, Type: TypeString} }
func (f *fakeSys) setUserX(name, val string)   { f.user[name] = Raw{Value: val, Type: TypeExpand} }
func (f *fakeSys) setMachine(name, val string) { f.machine[name] = Raw{Value: val, Type: TypeString} }
func (f *fakeSys) setMachineX(name, val string) {
	f.machine[name] = Raw{Value: val, Type: TypeExpand}
}

func (f *fakeSys) Env(s Scope) (map[string]Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.user
	if s == Machine {
		src = f.machine
	}
	out := make(map[string]Raw, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSys) SetUserVar(name, value string, typ uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failSet[name]; err != nil {
		return err
	}
	f.user[name] = Raw{Value: value, Type: typ}
	f.writes = append(f.writes, "set "+name)
	return nil
}

func (f *fakeSys) DeleteUserVar(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failSet[name]; err != nil {
		return err
	}
	delete(f.user, name)
	f.writes = append(f.writes, "del "+name)
	return nil
}

func (f *fakeSys) Broadcast() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts++
	return nil
}

func (f *fakeSys) Stat(p string) PathKind {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statCalls++
	return f.fs[fsKey(p)]
}

func (f *fakeSys) Getenv(n string) string {
	for k, v := range f.env {
		if strings.EqualFold(k, n) {
			return v
		}
	}
	return ""
}

func (f *fakeSys) BackupDir() (string, error) { return f.backupDir, nil }

func (f *fakeSys) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

var epoch = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

// mgr is a Manager whose clock starts at epoch and moves one minute per call.
func mgr(f *fakeSys) *Manager {
	t := epoch
	return &Manager{Sys: f, Now: func() time.Time { t = t.Add(time.Minute); return t }}
}

func scan(t *testing.T, f *fakeSys) Report {
	t.Helper()
	r, err := mgr(f).Scan()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// ids returns the id ("kind|scope|subject") of every finding.
func ids(r Report) []string {
	out := []string{}
	for _, f := range r.Findings {
		out = append(out, f.ID)
	}
	return out
}

func find(r Report, id string) (Finding, bool) {
	for _, f := range r.Findings {
		if f.ID == id {
			return f, true
		}
	}
	return Finding{}, false
}

func mustFind(t *testing.T, r Report, id string) Finding {
	t.Helper()
	f, ok := find(r, id)
	if !ok {
		t.Fatalf("no finding %q in %v", id, ids(r))
	}
	return f
}

func noFinding(t *testing.T, r Report, id string) {
	t.Helper()
	if _, ok := find(r, id); ok {
		t.Fatalf("unexpected finding %q in %v", id, ids(r))
	}
}

func countKind(r Report, kind string) int {
	n := 0
	for _, f := range r.Findings {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

var errBoom = errors.New("boom")
