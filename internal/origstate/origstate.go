// Package origstate remembers the state an item had before WinForge first changed it (the start
// mode of a service, whether a scheduled task was enabled), so the page can offer "Restore
// original". It is one small JSON file under the app-data folder, written atomically.
package origstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const version = 1

type file struct {
	Version int               `json:"version"`
	Items   map[string]string `json:"items"`
}

// Store keeps the original value of every item WinForge changed, by lower-case id.
type Store struct {
	Path string
	mu   sync.Mutex
}

// DefaultPath is <config dir>\WinForge\<name>, for example %AppData%\WinForge\services-original.json.
func DefaultPath(name string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinForge", name), nil
}

func key(id string) string { return strings.ToLower(id) }

// load reads the file. A missing file is empty. A damaged file is moved aside (to Path+".damaged")
// and treated as empty, so one bad write never blocks the feature and never loses evidence.
func (s *Store) load() (map[string]string, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil || f.Version != version || f.Items == nil {
		_ = os.Remove(s.Path + ".damaged")
		if err := os.Rename(s.Path, s.Path+".damaged"); err != nil {
			return nil, err
		}
		return map[string]string{}, nil
	}
	return f.Items, nil
}

func (s *Store) save(items map[string]string) error {
	if len(items) == 0 {
		if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(file{Version: version, Items: items}, "", "  ")
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

// All returns a copy of everything remembered.
func (s *Store) All() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(items))
	for k, v := range items {
		out[k] = v
	}
	return out, nil
}

// Get returns the remembered original of an item.
func (s *Store) Get(id string) (string, bool, error) {
	all, err := s.All()
	if err != nil {
		return "", false, err
	}
	v, ok := all[key(id)]
	return v, ok, nil
}

// Remember stores the original of an item unless one is already stored: the first change is the
// one that counts. added tells whether this call wrote it.
func (s *Store) Remember(id, value string) (added bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return false, err
	}
	if _, ok := items[key(id)]; ok {
		return false, nil
	}
	items[key(id)] = value
	return true, s.save(items)
}

// Forget drops what is remembered about an item.
func (s *Store) Forget(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := items[key(id)]; !ok {
		return nil
	}
	delete(items, key(id))
	return s.save(items)
}
