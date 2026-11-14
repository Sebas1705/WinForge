package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileIsTheDefaultConfig(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "WinForge", "config.json")}
	c, err := s.Load()
	if err != nil || c != (Config{}) {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestSavedConfigComesBack(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "WinForge", "config.json")}
	want := Config{ProfilesDir: `D:\OneDrive\WinForge profiles`}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := (&Store{Path: s.Path}).Load()
	if err != nil || got != want {
		t.Fatalf("%+v %v", got, err)
	}
	if left, _ := filepath.Glob(s.Path + ".tmp"); len(left) != 0 {
		t.Fatalf("temporary file left behind: %v", left)
	}
}

func TestSavingTheDefaultRemovesTheFile(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "config.json")}
	if err := s.Save(Config{ProfilesDir: "/x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
		t.Fatal("the default config needs no file")
	}
	if err := s.Save(Config{}); err != nil {
		t.Fatalf("removing what is not there is not an error: %v", err)
	}
}

func TestADamagedFileIsReportedNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Store{Path: path}).Load(); err == nil || !strings.Contains(err.Error(), "damaged") || !strings.Contains(err.Error(), path) {
		t.Fatalf("it must say the file is damaged and where it is: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "{oops" {
		t.Fatal("loading must not touch the file")
	}
}
