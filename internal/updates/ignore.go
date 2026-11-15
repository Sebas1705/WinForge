// Package updates remembers which app updates a person chose not to install:
// never, only this version, or not for a while. It decides nothing by itself; the
// app asks whether an available update is hidden right now.
package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Mode says how long an update stays hidden.
type Mode string

const (
	// Always hides every update of the app until the person stops ignoring it.
	Always Mode = "always"
	// Version hides one specific version; a newer one shows again.
	Version Mode = "version"
	// Until hides updates until a date.
	Until Mode = "until"
)

// MaxPauseDays bounds a pause, so "until" cannot silently become "always".
const MaxPauseDays = 365

// Rule is one app's choice.
type Rule struct {
	Mode    Mode      `json:"mode"`
	Version string    `json:"version,omitempty"` // for Version: the version that is skipped
	Until   time.Time `json:"until,omitempty"`   // for Until: when the update shows again
}

// Validate rejects a rule that could not mean anything.
func (r Rule) Validate() error {
	switch r.Mode {
	case Always:
		return nil
	case Version:
		if r.Version == "" {
			return errors.New("skipping a version needs the version")
		}
		return nil
	case Until:
		if r.Until.IsZero() {
			return errors.New("pausing needs an end date")
		}
		return nil
	}
	return fmt.Errorf("unknown way to ignore an update: %q", r.Mode)
}

// Hides reports whether the rule hides the given available version at this moment.
// An unknown or invalid rule hides nothing, so a damaged entry never freezes an app
// without the person having asked for it.
func (r Rule) Hides(available string, now time.Time) bool {
	if r.Validate() != nil {
		return false
	}
	switch r.Mode {
	case Always:
		return true
	case Version:
		return available == r.Version
	case Until:
		return now.Before(r.Until)
	}
	return false
}

// Store keeps the rules in one JSON file, written atomically.
type Store struct {
	Path string
	mu   sync.Mutex
}

// DefaultStore is %AppData%\WinForge\ignored-updates.json.
func DefaultStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(base, "WinForge", "ignored-updates.json")}, nil
}

// Load returns the rules by catalog id. A missing file is no rules. An unreadable one is
// an error, never "no rules": treating a damaged list as empty would update apps the
// person asked to keep as they are.
func (s *Store) Load() (map[string]Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() (map[string]Rule, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Rule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Rules map[string]Rule `json:"rules"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s is damaged (%v); fix or delete it", s.Path, err)
	}
	if doc.Rules == nil {
		doc.Rules = map[string]Rule{}
	}
	return doc.Rules, nil
}

func (s *Store) save(rules map[string]Rule) error {
	if len(rules) == 0 {
		if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(struct {
		Rules map[string]Rule `json:"rules"`
	}{rules}, "", "  ")
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

// Set stores the rule for an app, replacing any earlier one.
func (s *Store) Set(id string, r Rule) error {
	if id == "" {
		return errors.New("no app given")
	}
	if err := r.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rules, err := s.load()
	if err != nil {
		return err
	}
	rules[id] = r
	return s.save(rules)
}

// Remove stops ignoring an app. An app that was not ignored is not an error.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rules, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := rules[id]; !ok {
		return nil
	}
	delete(rules, id)
	return s.save(rules)
}
