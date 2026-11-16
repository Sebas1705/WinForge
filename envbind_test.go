package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/envdoctor"
)

// fakeEnv is an in-memory machine for the Environment page: no registry, no disk but a temp folder.
type fakeEnv struct {
	mu         sync.Mutex
	user       map[string]envdoctor.Raw
	machine    map[string]envdoctor.Raw
	dirs       map[string]bool
	dir        string
	writes     []string
	broadcasts int
	statCalls  int
}

func newFakeEnv(t *testing.T) *fakeEnv {
	return &fakeEnv{
		user:    map[string]envdoctor.Raw{},
		machine: map[string]envdoctor.Raw{},
		dirs:    map[string]bool{},
		dir:     t.TempDir(),
	}
}

func (f *fakeEnv) Env(s envdoctor.Scope) (map[string]envdoctor.Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.user
	if s == envdoctor.Machine {
		src = f.machine
	}
	out := map[string]envdoctor.Raw{}
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}

func (f *fakeEnv) SetUserVar(n, v string, typ uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user[n] = envdoctor.Raw{Value: v, Type: typ}
	f.writes = append(f.writes, "set "+n)
	return nil
}

func (f *fakeEnv) DeleteUserVar(n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.user, n)
	f.writes = append(f.writes, "del "+n)
	return nil
}

func (f *fakeEnv) Broadcast() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts++
	return nil
}

func (f *fakeEnv) Stat(p string) envdoctor.PathKind {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statCalls++
	if f.dirs[strings.ToLower(strings.TrimRight(p, `\`))] {
		return envdoctor.Dir
	}
	return envdoctor.Missing
}

func (f *fakeEnv) Getenv(string) string       { return "" }
func (f *fakeEnv) BackupDir() (string, error) { return f.dir, nil }
func (f *fakeEnv) writeCount() int            { f.mu.Lock(); defer f.mu.Unlock(); return len(f.writes) }
func (f *fakeEnv) userValue(n string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.user[n].Value
}

func envApp(t *testing.T) (*App, *fakeEnv) {
	t.Helper()
	a := testApp(t)
	f := newFakeEnv(t)
	f.dirs[`c:\keep`] = true
	f.user["Path"] = envdoctor.Raw{Value: `C:\keep;C:\gone;C:\KEEP\;rel`, Type: envdoctor.TypeExpand}
	f.user["JAVA_HOME"] = envdoctor.Raw{Value: `C:\jdk-gone`, Type: envdoctor.TypeString}
	f.machine["Path"] = envdoctor.Raw{Value: `C:\machine-gone`, Type: envdoctor.TypeString}
	a.envSys = f
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { now = now.Add(time.Minute); return now }
	return a, f
}

func TestEnvScanReportsAndWritesNothing(t *testing.T) {
	a, f := envApp(t)
	rep, err := a.EnvScan()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]envdoctor.Finding{}
	for _, fd := range rep.Findings {
		ids[fd.ID] = fd
	}
	for _, id := range []string{`path-missing|user|C:\gone`, `path-duplicate|user|C:\KEEP\`, `path-relative|user|rel`, `path-missing|machine|C:\machine-gone`, `home-missing|user|JAVA_HOME`} {
		if _, ok := ids[id]; !ok {
			t.Errorf("missing %q in %v", id, ids)
		}
	}
	if f.writeCount() != 0 || f.broadcasts != 0 {
		t.Error("scanning writes nothing")
	}
	if rep.Findings == nil || rep.Tools == nil || rep.Vars == nil {
		t.Errorf("%+v", rep)
	}
}

func TestEnvFixOnlyAfterAScan(t *testing.T) {
	a, f := envApp(t)
	if _, err := a.EnvFix([]string{`path-missing|user|C:\gone`}); err == nil || !strings.Contains(err.Error(), "not in the last scan") {
		t.Fatalf("before any scan: %v", err)
	}
	if f.writeCount() != 0 {
		t.Error("nothing written")
	}
}

func TestEnvFixRefusesUnlistedAndNonFixableIDs(t *testing.T) {
	a, f := envApp(t)
	if _, err := a.EnvScan(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		``,
		`path-missing|user|C:\never-listed`,
		`path-missing|machine|C:\machine-gone`, // listed but machine: not fixable
		`path-relative|user|rel`,               // listed but not fixable
		`path-missing|user|c:\gone`,            // other casing is another id
		`../../x`,
	} {
		res, err := a.EnvFix([]string{id})
		if err == nil || !strings.Contains(err.Error(), "not in the last scan") {
			t.Errorf("%q must be refused, got %+v %v", id, res, err)
		}
		if res.Failed == nil {
			t.Errorf("failed is never nil")
		}
	}
	if f.writeCount() != 0 {
		t.Errorf("nothing written: %v", f.writes)
	}
}

func TestEnvFixOneUnknownIDRefusesTheWholeRequest(t *testing.T) {
	a, f := envApp(t)
	if _, err := a.EnvScan(); err != nil {
		t.Fatal(err)
	}
	before := f.userValue("Path")
	_, err := a.EnvFix([]string{`path-missing|user|C:\gone`, `home-missing|user|JAVA_HOME`, `path-missing|user|C:\other`})
	if err == nil {
		t.Fatal("must refuse")
	}
	if f.writeCount() != 0 || f.broadcasts != 0 || f.userValue("Path") != before || f.userValue("JAVA_HOME") == "" {
		t.Errorf("nothing is written when one id is unknown: %v", f.writes)
	}
	if es, _ := os.ReadDir(f.dir); len(es) != 0 {
		t.Error("no backup either")
	}
	// The listed ones still work afterwards: a refusal does not spoil the scan.
	res, err := a.EnvFix([]string{`path-missing|user|C:\gone`})
	if err != nil || res.Applied != 1 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestEnvFixNothingChosen(t *testing.T) {
	a, _ := envApp(t)
	_, _ = a.EnvScan()
	if _, err := a.EnvFix(nil); err == nil {
		t.Error("an empty request is an error")
	}
}

func TestEnvFixThenBackupsThenRestore(t *testing.T) {
	a, f := envApp(t)
	if _, err := a.EnvScan(); err != nil {
		t.Fatal(err)
	}
	res, err := a.EnvFix([]string{`path-missing|user|C:\gone`, `path-duplicate|user|C:\KEEP\`, `home-missing|user|JAVA_HOME`})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 3 || len(res.Failed) != 0 || res.BackupID == "" {
		t.Fatalf("%+v", res)
	}
	if got := f.userValue("Path"); got != `C:\keep;rel` || f.user["Path"].Type != envdoctor.TypeExpand {
		t.Errorf("PATH = %+v", f.user["Path"])
	}
	if _, has := f.user["JAVA_HOME"]; has {
		t.Error("JAVA_HOME unset")
	}
	if f.machine["Path"].Value != `C:\machine-gone` {
		t.Error("machine untouched")
	}
	if f.broadcasts != 1 {
		t.Errorf("broadcasts %d", f.broadcasts)
	}

	list, err := a.EnvBackups()
	if err != nil || len(list) != 1 || list[0].ID != res.BackupID || !reflect.DeepEqual(list[0].Changed, []string{"Path", "JAVA_HOME"}) {
		t.Fatalf("%+v %v", list, err)
	}

	// The fix invalidates the scan: its ids cannot be replayed.
	if _, err := a.EnvFix([]string{`path-missing|user|C:\gone`}); err == nil {
		t.Error("ids are single use until the next scan")
	}

	if err := a.EnvRestore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	if f.userValue("Path") != `C:\keep;C:\gone;C:\KEEP\;rel` || f.userValue("JAVA_HOME") != `C:\jdk-gone` {
		t.Errorf("restored: %+v", f.user)
	}
	if f.broadcasts != 2 {
		t.Errorf("broadcasts %d", f.broadcasts)
	}
}

func TestEnvRestoreOnlyListedBackupIDs(t *testing.T) {
	a, f := envApp(t)
	_, _ = a.EnvScan()
	res, _ := a.EnvFix([]string{`path-missing|user|C:\gone`})
	f.writes = nil
	outside := f.dir + `-outside.json`
	_ = os.WriteFile(outside, []byte(`{"vars":[{"name":"X","value":"1","type":1}]}`), 0o600)
	for _, id := range []string{"", "nope", "../x", outside, strings.TrimSuffix(outside, ".json"), f.dir + `\` + res.BackupID, res.BackupID + ".json", "20990101T000000Z"} {
		if err := a.EnvRestore(id); err == nil || !strings.Contains(err.Error(), "not in the list") {
			t.Errorf("%q must be refused, got %v", id, err)
		}
	}
	if f.writeCount() != 0 {
		t.Errorf("nothing written: %v", f.writes)
	}
}

func TestEnvBackupsEmptyIsNotNull(t *testing.T) {
	a, _ := envApp(t)
	l, err := a.EnvBackups()
	if err != nil || l == nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
}

func TestEnvChangesAreOneAtATime(t *testing.T) {
	a, f := envApp(t)
	if _, err := a.EnvScan(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.EnvFix([]string{`path-missing|user|C:\gone`})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, errEnvNotListed) {
			t.Errorf("unexpected %v", err)
		}
	}
	if ok < 1 {
		t.Error("one of them applied")
	}
	if got := f.userValue("Path"); got != `C:\keep;C:\KEEP\;rel` {
		t.Errorf("the segment was removed exactly once: %q", got)
	}
}
