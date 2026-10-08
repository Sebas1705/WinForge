package games_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Sebas1705/WinForge/internal/games"
)

func TestGamesFolderIsRememberedAndValidated(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg", "games.json")
	def := games.DefaultRoot(filepath.Join(dir, "Documents"))
	if got := games.LoadRoot(cfg, def); got != def || filepath.Base(def) != "WinForge Games" {
		t.Fatalf("default: %q", got)
	}
	chosen := filepath.Join(dir, "My Games")
	if err := os.MkdirAll(chosen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := games.SaveRoot(cfg, chosen); err != nil {
		t.Fatal(err)
	}
	if got := games.LoadRoot(cfg, def); got != chosen {
		t.Fatalf("saved: %q", got)
	}
	if err := games.SaveRoot(cfg, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("a folder that does not exist must be refused")
	}
	if err := games.SaveRoot(cfg, "relative/path"); err == nil {
		t.Fatal("a relative path must be refused")
	}
	file := filepath.Join(dir, "a-file.txt")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if err := games.SaveRoot(cfg, file); err == nil {
		t.Fatal("a file is not a folder")
	}
	if got := games.LoadRoot(cfg, def); got != chosen {
		t.Fatal("a refused folder must not replace the saved one")
	}
	_ = os.WriteFile(cfg, []byte("{not json"), 0o644)
	if got := games.LoadRoot(cfg, def); got != def {
		t.Fatalf("a damaged file falls back to the default: %q", got)
	}
	_ = os.WriteFile(cfg, []byte(`{"root": "relative"}`), 0o644)
	if got := games.LoadRoot(cfg, def); got != def {
		t.Fatalf("a relative saved path is ignored: %q", got)
	}
}
