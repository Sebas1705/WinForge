package origstate

import (
	"os"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	return &Store{Path: filepath.Join(t.TempDir(), "sub", "orig.json")}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s := newStore(t)
	all, err := s.All()
	if err != nil || len(all) != 0 {
		t.Fatalf("All() = %v, %v", all, err)
	}
	if _, ok, _ := s.Get("x"); ok {
		t.Fatal("nothing should be remembered")
	}
}

func TestRememberKeepsTheFirstValueAndIsCaseInsensitive(t *testing.T) {
	s := newStore(t)
	added, err := s.Remember("MyService", "delayed")
	if err != nil || !added {
		t.Fatalf("first Remember = %v, %v", added, err)
	}
	added, err = s.Remember("myservice", "manual")
	if err != nil || added {
		t.Fatalf("second Remember = %v, %v; must not overwrite", added, err)
	}
	v, ok, _ := s.Get("MYSERVICE")
	if !ok || v != "delayed" {
		t.Fatalf("Get = %q, %v", v, ok)
	}
	// A new Store on the same file sees it: it is on disk.
	v, ok, _ = (&Store{Path: s.Path}).Get("myservice")
	if !ok || v != "delayed" {
		t.Fatalf("reloaded = %q, %v", v, ok)
	}
}

func TestForgetRemovesAndDeletesTheEmptyFile(t *testing.T) {
	s := newStore(t)
	_, _ = s.Remember("a", "auto")
	_, _ = s.Remember("b", "manual")
	if err := s.Forget("a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get("a"); ok {
		t.Fatal("a should be gone")
	}
	if err := s.Forget("b"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatalf("an empty store should leave no file: %v", err)
	}
	if err := s.Forget("never-there"); err != nil {
		t.Fatalf("forgetting nothing is fine: %v", err)
	}
}

func TestWriteIsAtomicNoTempLeft(t *testing.T) {
	s := newStore(t)
	_, _ = s.Remember("a", "auto")
	if _, err := os.Stat(s.Path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestDamagedFileIsMovedAsideAndStartsEmpty(t *testing.T) {
	for name, content := range map[string]string{
		"garbage":       "{not json",
		"wrong version": `{"version":99,"items":{"a":"auto"}}`,
		"no items":      `{"version":1}`,
		"empty":         ``,
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.Path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			all, err := s.All()
			if err != nil || len(all) != 0 {
				t.Fatalf("All() = %v, %v", all, err)
			}
			if b, err := os.ReadFile(s.Path + ".damaged"); err != nil || string(b) != content {
				t.Fatalf("damaged copy missing or changed: %q, %v", b, err)
			}
			if added, err := s.Remember("a", "auto"); err != nil || !added {
				t.Fatalf("a damaged file must not block new changes: %v, %v", added, err)
			}
		})
	}
}
