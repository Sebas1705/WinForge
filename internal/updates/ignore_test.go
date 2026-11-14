package updates

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func TestEachWayOfIgnoringHidesWhatItShould(t *testing.T) {
	cases := []struct {
		name      string
		rule      Rule
		available string
		hidden    bool
	}{
		{"always hides any version", Rule{Mode: Always}, "9.9", true},
		{"a skipped version is hidden", Rule{Mode: Version, Version: "2.0"}, "2.0", true},
		{"a newer version shows again", Rule{Mode: Version, Version: "2.0"}, "2.1", false},
		{"an older available version is not the skipped one", Rule{Mode: Version, Version: "2.0"}, "1.9", false},
		{"a pause hides until its date", Rule{Mode: Until, Until: now.Add(time.Hour)}, "3.0", true},
		{"a pause ends on its date", Rule{Mode: Until, Until: now}, "3.0", false},
		{"an expired pause shows again", Rule{Mode: Until, Until: now.Add(-time.Second)}, "3.0", false},
	}
	for _, c := range cases {
		if got := c.rule.Hides(c.available, now); got != c.hidden {
			t.Errorf("%s: Hides = %v, want %v", c.name, got, c.hidden)
		}
	}
}

func TestADamagedRuleNeverFreezesAnApp(t *testing.T) {
	for _, r := range []Rule{{}, {Mode: "forever"}, {Mode: Version}, {Mode: Until}} {
		if r.Validate() == nil {
			t.Errorf("%+v should not validate", r)
		}
		if r.Hides("1.0", now) {
			t.Errorf("%+v is invalid and must hide nothing", r)
		}
	}
}

func TestStoreKeepsRulesAcrossReopening(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "sub", "ignored.json")}
	if rules, err := s.Load(); err != nil || len(rules) != 0 {
		t.Fatalf("no file means no rules: %v %v", rules, err)
	}
	if err := s.Set("git", Rule{Mode: Always}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("vlc", Rule{Mode: Version, Version: "3.1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("git", Rule{Mode: Until, Until: now.Add(48 * time.Hour)}); err != nil { // replaces
		t.Fatal(err)
	}
	again := &Store{Path: s.Path}
	rules, err := again.Load()
	if err != nil || len(rules) != 2 || rules["git"].Mode != Until || !rules["git"].Until.Equal(now.Add(48*time.Hour)) || rules["vlc"].Version != "3.1" {
		t.Fatalf("%+v %v", rules, err)
	}
	if err := again.Remove("git"); err != nil {
		t.Fatal(err)
	}
	if err := again.Remove("never-ignored"); err != nil {
		t.Fatalf("removing what was not ignored is not an error: %v", err)
	}
	if err := again.Remove("vlc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("no rules left, no file left")
	}
	if matches, _ := filepath.Glob(s.Path + "*"); len(matches) != 0 {
		t.Fatalf("temporary files left behind: %v", matches)
	}
}

func TestAnUnreadableFileIsAnErrorNotAnEmptyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignored.json")
	if err := os.WriteFile(path, []byte(`{"rules": [`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path}
	if _, err := s.Load(); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("a damaged list must say so: %v", err)
	}
	// and it is never overwritten behind the person's back
	if err := s.Set("git", Rule{Mode: Always}); err == nil {
		t.Fatal("setting on top of a damaged file must fail")
	}
	if b, _ := os.ReadFile(path); string(b) != `{"rules": [` {
		t.Fatalf("the damaged file was changed: %q", b)
	}
}

func TestSetRefusesNonsense(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "i.json")}
	if err := s.Set("", Rule{Mode: Always}); err == nil {
		t.Fatal("an app is needed")
	}
	if err := s.Set("git", Rule{Mode: Version}); err == nil {
		t.Fatal("a skipped version needs a version")
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("nothing should have been written")
	}
}

func TestConcurrentChangesAreNotLost(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "i.json")}
	var wg sync.WaitGroup
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Set(id, Rule{Mode: Always}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	rules, err := s.Load()
	if err != nil || len(rules) != len(ids) {
		t.Fatalf("%d of %d rules survived: %v", len(rules), len(ids), err)
	}
}
