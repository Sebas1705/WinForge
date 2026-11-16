package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStore keeps the history in one JSON file, written atomically and capped at Max entries.
// A file that cannot be parsed is moved aside (quarantined) and the history starts again from
// empty, so a damaged file never stops anything from being installed.
type FileStore struct {
	Path string
	Max  int              // 0 means MaxEntries
	Now  func() time.Time // names the quarantined file; nil means time.Now

	mu          sync.Mutex
	quarantined string
}

// DefaultStore is %AppData%\WinForge\history.json.
func DefaultStore() (*FileStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &FileStore{Path: filepath.Join(base, "WinForge", "history.json")}, nil
}

type fileDoc struct {
	Entries []Entry `json:"entries"`
}

func (s *FileStore) max() int {
	if s.Max > 0 {
		return s.Max
	}
	return MaxEntries
}

// Quarantined is where a damaged file was moved to, "" when none was.
func (s *FileStore) Quarantined() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quarantined
}

// load reads the file. A missing file is an empty history; a file that does not parse is moved
// aside and also reads as empty; any other read problem is an error.
func (s *FileStore) load() ([]Entry, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc fileDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		now := time.Now
		if s.Now != nil {
			now = s.Now
		}
		aside := fmt.Sprintf("%s.damaged-%s", s.Path, now().UTC().Format("20060102T150405.000"))
		if rerr := os.Rename(s.Path, aside); rerr != nil {
			return nil, fmt.Errorf("%s is damaged (%v) and could not be moved aside: %w", s.Path, err, rerr)
		}
		s.quarantined = aside
		return nil, nil
	}
	out := doc.Entries[:0]
	for _, e := range doc.Entries {
		if e.Valid() {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *FileStore) save(entries []Entry) error {
	if n := s.max(); len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	b, err := json.MarshalIndent(fileDoc{Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	// Written beside and renamed, so a crash mid-write cannot leave half a file.
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Add implements Store.
func (s *FileStore) Add(entries ...Entry) error {
	if len(entries) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return err
	}
	return s.save(append(all, entries...))
}

// All implements Store.
func (s *FileStore) All() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// SetTo implements Store.
func (s *FileStore) SetTo(to map[string]string) error {
	if len(to) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return err
	}
	changed := false
	for i := range all {
		if v, ok := to[all[i].ID]; ok && all[i].To != v {
			all[i].To, changed = v, true
		}
	}
	if !changed {
		return nil
	}
	return s.save(all)
}

// MemStore is an in-memory Store, for tests. Fail makes every call return that error.
type MemStore struct {
	mu      sync.Mutex
	Entries []Entry
	Max     int
	Fail    error
}

// Add implements Store.
func (m *MemStore) Add(entries ...Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.Entries = append(m.Entries, entries...)
	n := m.Max
	if n <= 0 {
		n = MaxEntries
	}
	if len(m.Entries) > n {
		m.Entries = m.Entries[len(m.Entries)-n:]
	}
	return nil
}

// All implements Store.
func (m *MemStore) All() ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	return append([]Entry(nil), m.Entries...), nil
}

// SetTo implements Store.
func (m *MemStore) SetTo(to map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	for i := range m.Entries {
		if v, ok := to[m.Entries[i].ID]; ok {
			m.Entries[i].To = v
		}
	}
	return nil
}
