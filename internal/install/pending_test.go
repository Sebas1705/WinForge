package install_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Sebas1705/WinForge/internal/install"
)

func TestPendingSurvivesARestartAndClearsWhenEmpty(t *testing.T) {
	s := &install.PendingStore{Path: filepath.Join(t.TempDir(), "sub", "pending.json")}
	if s.Load() != nil {
		t.Fatal("nothing stored yet")
	}
	steps := []install.Step{{Kind: install.StepApp, ID: "git", Name: "Git"}, {Kind: install.StepRecipe, ID: "wsl", Name: "WSL"}}
	if err := s.Save(install.Pending{Title: "Dev", Steps: steps}); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if got == nil || got.Title != "Dev" || len(got.Steps) != 2 {
		t.Fatalf("%+v", got)
	}
	rest := install.Without(steps, map[string]bool{install.StepKey(steps[0]): true})
	if len(rest) != 1 || rest[0].ID != "wsl" {
		t.Fatalf("%+v", rest)
	}
	if err := s.Save(install.Pending{Title: "Dev", Steps: nil}); err != nil || s.Load() != nil {
		t.Fatal("saving no steps must clear the stored run")
	}
	if err := s.Clear(); err != nil {
		t.Fatal("clearing nothing is fine:", err)
	}
}

func TestCorruptPendingFileIsIgnored(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pending.json")
	s := &install.PendingStore{Path: p}
	if err := s.Save(install.Pending{Steps: []install.Step{{Kind: install.StepApp, ID: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(p, "{not json"); err != nil {
		t.Fatal(err)
	}
	if s.Load() != nil {
		t.Fatal("a half-written file must not crash or resume garbage")
	}
}

func writeFile(p, s string) error { return os.WriteFile(p, []byte(s), 0o644) }
