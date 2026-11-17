package main

import (
	"encoding/binary"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/privacy"
)

// fakeReg is an in-memory current-user registry for the Privacy page: no real registry is touched.
type fakeReg struct {
	mu         sync.Mutex
	vals       map[string]privacy.Value
	keys       map[string]bool
	dir        string
	writes     int
	broadcasts int
}

func newFakeReg(t *testing.T) *fakeReg {
	return &fakeReg{vals: map[string]privacy.Value{}, keys: map[string]bool{}, dir: t.TempDir()}
}

func rk(p, n string) string { return strings.ToLower(p) + "|" + strings.ToLower(n) }

func (f *fakeReg) addKeys(p string) {
	for ; p != ""; p = p[:max(strings.LastIndex(p, `\`), 0)] {
		f.keys[strings.ToLower(p)] = true
	}
}

func (f *fakeReg) putDWord(p, n string, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	f.addKeys(p)
	f.vals[rk(p, n)] = privacy.Value{Exists: true, Type: privacy.TypeDWord, Data: b}
}

func (f *fakeReg) Get(p, n string) (privacy.Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[rk(p, n)], nil
}
func (f *fakeReg) KeyExists(p string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keys[strings.ToLower(p)], nil
}
func (f *fakeReg) Set(p, n string, typ uint32, d []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addKeys(p)
	f.vals[rk(p, n)] = privacy.Value{Exists: true, Type: typ, Data: append([]byte(nil), d...)}
	f.writes++
	return nil
}
func (f *fakeReg) Delete(p, n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.vals, rk(p, n))
	f.writes++
	return nil
}
func (f *fakeReg) DeleteKeyIfEmpty(p string) error { return nil }
func (f *fakeReg) Broadcast() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts++
	return nil
}
func (f *fakeReg) BackupDir() (string, error) { return f.dir, nil }

func newPrivacyApp(f *fakeReg) *App {
	a := &App{}
	a.privacySys = f
	return a
}

const advKey = `Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`

func TestPrivacyListThenApplyOnlyListedIDs(t *testing.T) {
	f := newFakeReg(t)
	a := newPrivacyApp(f)

	// Nothing was listed yet: even a real id is refused.
	if _, err := a.PrivacyApply([]string{"advertising-id"}); err != errPrivacyNotListed {
		t.Fatalf("before listing: %v", err)
	}
	list, err := a.PrivacyList()
	if err != nil || len(list) != 11 {
		t.Fatalf("%d tweaks, %v", len(list), err)
	}
	for _, v := range list {
		if v.State != privacy.StateDefault {
			t.Errorf("%s: %s", v.ID, v.State)
		}
	}

	// One unknown id refuses the whole request, for both directions, and nothing is written.
	for _, call := range []func([]string) (privacy.Result, error){a.PrivacyApply, a.PrivacyReset} {
		if _, err := call([]string{"advertising-id", "../../evil"}); err != errPrivacyNotListed {
			t.Errorf("unknown id: %v", err)
		}
		if _, err := call(nil); err != errPrivacyNothing {
			t.Errorf("empty: %v", err)
		}
	}
	if f.writes != 0 {
		t.Fatalf("%d writes after refused requests", f.writes)
	}
	if e, _ := os.ReadDir(f.dir); len(e) != 0 {
		t.Fatal("a backup was written for a refused request")
	}

	res, err := a.PrivacyApply([]string{"advertising-id"})
	if err != nil || res.Applied != 1 || res.BackupID == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if f.broadcasts != 1 {
		t.Errorf("%d broadcasts", f.broadcasts)
	}
}

func TestPrivacyRestoreOnlyListedBackups(t *testing.T) {
	f := newFakeReg(t)
	f.putDWord(advKey, "Enabled", 1)
	a := newPrivacyApp(f)
	if _, err := a.PrivacyList(); err != nil {
		t.Fatal(err)
	}
	res, _ := a.PrivacyApply([]string{"advertising-id"})
	for _, bad := range []string{"", "../x", "20990101T000000Z", `C:\Windows\x`} {
		if _, err := a.PrivacyRestore(bad); err != errPrivacyNoBackup {
			t.Errorf("Restore(%q): %v", bad, err)
		}
	}
	f.putDWord(advKey, "Enabled", 3) // edited by someone else
	rr, err := a.PrivacyRestore(res.BackupID)
	if err != nil || !reflect.DeepEqual(rr.ChangedSince, []string{"advertising-id"}) || len(rr.Restored) != 1 {
		t.Fatalf("%+v %v", rr, err)
	}
	if v := f.vals[rk(advKey, "Enabled")]; binary.LittleEndian.Uint32(v.Data) != 1 {
		t.Error("not restored")
	}
	b, _ := a.PrivacyBackups()
	if len(b) != 1 || b[0].Restored == nil {
		t.Errorf("%+v", b)
	}
}

// Many changes at once, as double clicks or two windows would send them: each is whole, every
// backup is a separate readable file, and every value can still be undone.
func TestPrivacyConcurrentChangesDoNotCorruptBackups(t *testing.T) {
	f := newFakeReg(t)
	a := newPrivacyApp(f)
	list, _ := a.PrivacyList()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := list[i%len(list)].ID
			if i%2 == 0 {
				_, _ = a.PrivacyApply([]string{id})
			} else {
				_, _ = a.PrivacyReset([]string{id})
			}
		}(i)
	}
	wg.Wait()
	backups, err := a.PrivacyBackups()
	if err != nil || len(backups) == 0 || len(backups) > 10 {
		t.Fatalf("%d backups, %v", len(backups), err)
	}
	seen := map[string]bool{}
	for _, b := range backups {
		if seen[b.ID] {
			t.Errorf("duplicate id %s", b.ID)
		}
		seen[b.ID] = true
		if _, err := a.PrivacyRestore(b.ID); err != nil {
			t.Errorf("restore %s: %v", b.ID, err)
		}
	}
	if e, _ := os.ReadDir(f.dir); len(e) != len(backups) {
		t.Errorf("%d files for %d backups (a temp file was left?)", len(e), len(backups))
	}
}
