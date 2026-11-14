// Package appconfig holds the few settings that live on disk next to the profiles rather than in the
// window: today, where the profiles are kept.
package appconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is what the person chose. A zero value means every default.
type Config struct {
	// ProfilesDir is a folder the person picked for their profiles, for example one inside OneDrive.
	// Empty means %AppData%\WinForge\profiles.
	ProfilesDir string `json:"profilesDir,omitempty"`
}

// Store keeps the config in one JSON file, written atomically.
type Store struct{ Path string }

// DefaultStore is %AppData%\WinForge\config.json.
func DefaultStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(base, "WinForge", "config.json")}, nil
}

// Load returns the config. A missing file is the default config; a damaged one is an error that
// says where it is, so it can be fixed instead of being silently replaced.
func (s *Store) Load() (Config, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s is damaged (%v); fix or delete it", s.Path, err)
	}
	return c, nil
}

// Save stores the config. The default config removes the file, so there is nothing left to go stale.
func (s *Store) Save(c Config) error {
	if c == (Config{}) {
		if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
