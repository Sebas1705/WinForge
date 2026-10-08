package games

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is where the person keeps their games.
type Config struct {
	Root string `json:"root"`
}

// ConfigPath is %AppData%\WinForge\games.json.
func ConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinForge", "games.json"), nil
}

// DefaultRoot is "WinForge Games" inside Documents.
func DefaultRoot(documents string) string {
	return filepath.Join(documents, "WinForge Games")
}

// LoadRoot reads the chosen games folder, or returns def when none was chosen
// or the file is unreadable.
func LoadRoot(cfgPath, def string) string {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return def
	}
	var c Config
	if json.Unmarshal(b, &c) != nil || c.Root == "" || !filepath.IsAbs(c.Root) {
		return def
	}
	return c.Root
}

// SaveRoot stores the games folder. It must be an existing folder, so a typo
// cannot send downloads somewhere unexpected.
func SaveRoot(cfgPath, root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("choose a full folder path")
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return errors.New("that folder does not exist")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(Config{Root: filepath.Clean(root)}, "", "  ")
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, cfgPath)
}
