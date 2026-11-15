package drift

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNothingChangedIsAnEmptyReport(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "2.50", "vscode": "1.99"}}
	r := Compare(base, []string{"git", "vscode"}, map[string]string{"git": "2.50", "vscode": "1.99"})
	if r.Count() != 0 || r.Removed == nil || r.Added == nil || r.Updated == nil {
		t.Fatalf("%+v (the lists must be empty, never null)", r)
	}
}

func TestARemovedProfileAppIsReported(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "2", "vscode": "1", "docker": "4"}}
	r := Compare(base, []string{"git", "vscode"}, map[string]string{"git": "2", "docker": "4"})
	if !reflect.DeepEqual(r.Removed, []string{"vscode"}) || len(r.Added) != 0 || len(r.Updated) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestAnAppThatWasNeverInTheProfileIsNotRemoved(t *testing.T) {
	// docker was installed at the baseline but is not part of the profile: removing it is not drift
	base := Baseline{Apps: map[string]string{"git": "2", "docker": "4"}}
	r := Compare(base, []string{"git"}, map[string]string{"git": "2"})
	if r.Count() != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestNewAppsOutsideTheProfileAreAdded(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "2"}}
	r := Compare(base, []string{"git"}, map[string]string{"git": "2", "zed": "1", "alpha": "3"})
	if !reflect.DeepEqual(r.Added, []string{"alpha", "zed"}) {
		t.Fatalf("sorted, so the list does not shuffle: %+v", r)
	}
}

func TestInstallingWhatTheProfileListsIsNotDrift(t *testing.T) {
	// vlc is in the profile but was not installed at the baseline; installing it completes the profile
	base := Baseline{Apps: map[string]string{"git": "2"}}
	r := Compare(base, []string{"git", "vlc"}, map[string]string{"git": "2", "vlc": "3"})
	if r.Count() != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestVersionsMoveOnlyWhenBothSidesKnowThem(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "2.50", "node": "", "py": "3.12", "go": "1.22"}}
	r := Compare(base, []string{"git", "node", "py", "go"}, map[string]string{"git": "2.51", "node": "20", "py": "", "go": "1.22"})
	want := []Change{{ID: "git", From: "2.50", To: "2.51"}}
	if !reflect.DeepEqual(r.Updated, want) {
		t.Fatalf("an unknown version is not a change: %+v", r.Updated)
	}
}

func TestOnlyProfileAppsCountAsUpdated(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "2", "docker": "4"}}
	r := Compare(base, []string{"git"}, map[string]string{"git": "2", "docker": "5"})
	if r.Count() != 0 {
		t.Fatalf("docker is not part of the profile: %+v", r)
	}
}

func TestEverythingAtOnceIsSorted(t *testing.T) {
	base := Baseline{Apps: map[string]string{"b": "1", "a": "1", "c": "1", "d": "1"}}
	r := Compare(base, []string{"a", "b", "c", "d"}, map[string]string{"a": "2", "d": "2", "n": "1", "m": "1"})
	if !reflect.DeepEqual(r.Removed, []string{"b", "c"}) || !reflect.DeepEqual(r.Added, []string{"m", "n"}) ||
		!reflect.DeepEqual(r.Updated, []Change{{"a", "1", "2"}, {"d", "1", "2"}}) || r.Count() != 6 {
		t.Fatalf("%+v", r)
	}
}

func TestACompareDoesNotChangeItsInputs(t *testing.T) {
	base := Baseline{Apps: map[string]string{"git": "1"}}
	installed := map[string]string{"git": "2", "x": "1"}
	prof := []string{"git"}
	Compare(base, prof, installed)
	if base.Apps["git"] != "1" || installed["git"] != "2" || len(installed) != 2 || len(prof) != 1 {
		t.Fatal("inputs were modified")
	}
}

var at = time.Date(2026, 11, 15, 10, 30, 0, 0, time.FixedZone("x", 3600))

func TestTheStoreRemembersABaselinePerProfile(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "WinForge", "applied.json")}
	if all, err := s.All(); err != nil || len(all) != 0 {
		t.Fatalf("no file means no baselines: %v %v", all, err)
	}
	if err := s.Set("rust", map[string]string{"git": "2", "rustup": ""}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("web", map[string]string{"node": "20"}, at); err != nil {
		t.Fatal(err)
	}
	b, ok, err := (&Store{Path: s.Path}).Get("rust")
	if err != nil || !ok || !b.At.Equal(at) || b.Apps["git"] != "2" || len(b.Apps) != 2 {
		t.Fatalf("%+v %v %v", b, ok, err)
	}
	if b.At.Location() != time.UTC {
		t.Fatal("the time is stored in UTC, so it means the same on every PC")
	}
	if _, ok, _ := s.Get("nope"); ok {
		t.Fatal("an unknown profile has no baseline")
	}
	if err := s.Set("rust", map[string]string{"git": "3"}, at.Add(time.Hour)); err != nil { // replaced, not merged
		t.Fatal(err)
	}
	if b, _, _ := s.Get("rust"); len(b.Apps) != 1 || b.Apps["git"] != "3" {
		t.Fatalf("a new baseline replaces the old: %+v", b)
	}
}

func TestTheStoreCopiesWhatItIsGiven(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "a.json")}
	m := map[string]string{"git": "1"}
	if err := s.Set("p", m, at); err != nil {
		t.Fatal(err)
	}
	m["git"] = "99"
	if b, _, _ := s.Get("p"); b.Apps["git"] != "1" {
		t.Fatal("changing the caller's map must not change what was stored")
	}
}

func TestForgettingABaselineLeavesNoFileWhenNoneRemain(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "a.json")}
	_ = s.Set("a", map[string]string{}, at)
	_ = s.Set("b", map[string]string{}, at)
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("ghost"); err != nil {
		t.Fatalf("forgetting what was not there is not an error: %v", err)
	}
	if _, err := os.Stat(s.Path); err != nil {
		t.Fatal("one baseline remains, so the file must too")
	}
	if err := s.Remove("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("none remain, no file")
	}
	if left, _ := filepath.Glob(s.Path + "*"); len(left) != 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

func TestADamagedFileIsReportedAndNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "applied.json")
	if err := os.WriteFile(path, []byte(`{"profiles": `), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path}
	if _, err := s.All(); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("%v", err)
	}
	if err := s.Set("p", map[string]string{"git": "1"}, at); err == nil {
		t.Fatal("must not write over a file it cannot read")
	}
	if err := s.Remove("p"); err == nil {
		t.Fatal("same for removing")
	}
	if b, _ := os.ReadFile(path); string(b) != `{"profiles": ` {
		t.Fatalf("the damaged file was changed: %q", b)
	}
}

func TestSetNeedsAProfile(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "a.json")}
	if err := s.Set("", map[string]string{}, at); err == nil {
		t.Fatal("a baseline belongs to a profile")
	}
}

func TestConcurrentWritesAreNotLost(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "a.json")}
	var wg sync.WaitGroup
	ids := []string{"a", "b", "c", "d", "e", "f"}
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Set(id, map[string]string{"git": "1"}, at); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if all, err := s.All(); err != nil || len(all) != len(ids) {
		t.Fatalf("%d of %d survived: %v", len(all), len(ids), err)
	}
}
