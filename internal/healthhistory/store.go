package healthhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps the history file. Load never fails because of the file's contents: a damaged file is
// set aside and the history starts again, so scanning is never held up by it.
type Store interface {
	Load() (File, error)
	Save(File) error
	Clear() error
}

// FileStore keeps the history in one JSON file, written atomically.
type FileStore struct{ Path string }

// DefaultStore is %AppData%\WinForge\health-history.json.
func DefaultStore() (*FileStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &FileStore{Path: filepath.Join(base, "WinForge", "health-history.json")}, nil
}

// Load returns the stored history. A missing file is an empty history. A file that is not valid
// JSON, or was written by a newer version, is renamed to <path>.damaged (replacing an older one)
// and an empty history is returned; only a failure to read the file at all is an error.
func (s *FileStore) Load() (File, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Version: fileVersion, Snapshots: []Snapshot{}}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil || f.Version > fileVersion {
		_ = os.Remove(s.Path + ".damaged")
		if rerr := os.Rename(s.Path, s.Path+".damaged"); rerr != nil {
			_ = os.Remove(s.Path) // never block the next save on a file we cannot move
		}
		return File{Version: fileVersion, Snapshots: []Snapshot{}}, nil
	}
	return Normalize(f), nil
}

// Save writes the file through a temporary file in the same folder and renames it over the old
// one, so a crash leaves either the old history or the new one, never half of one.
func (s *FileStore) Save(f File) error {
	f = Normalize(f)
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := out.Write(b); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Clear deletes the history (a damaged copy set aside earlier stays: it is not history).
func (s *FileStore) Clear() error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// MemStore is an in-memory Store for tests. LoadErr and SaveErr make it fail.
type MemStore struct {
	mu      sync.Mutex
	File    File
	LoadErr error
	SaveErr error
	Saves   int
}

func (m *MemStore) Load() (File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.LoadErr != nil {
		return File{}, m.LoadErr
	}
	return Normalize(m.File), nil
}

func (m *MemStore) Save(f File) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SaveErr != nil {
		return m.SaveErr
	}
	m.File = Normalize(f)
	m.Saves++
	return nil
}

func (m *MemStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.File = File{}
	return nil
}

// Service is the history behind the interface: one writer at a time.
type Service struct {
	Store Store
	mu    sync.Mutex
}

// Record adds a snapshot. A history that cannot be read or written returns an error that the
// caller logs; it must never fail a scan.
func (s *Service) Record(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.Store.Load()
	if err != nil {
		return fmt.Errorf("health history: %w", err)
	}
	return s.Store.Save(Add(f, snap))
}

// View is what the page shows: the snapshots (oldest first) and the plain statements about them.
type View struct {
	Snapshots  []Snapshot  `json:"snapshots"`
	Statements []Statement `json:"statements"`
	// Enough is true when there are two snapshots at least a day apart, so trends can be said.
	Enough bool `json:"enough"`
}

// View reads the history and works out its trends.
func (s *Service) View() (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.Store.Load()
	if err != nil {
		return View{Snapshots: []Snapshot{}, Statements: []Statement{}}, err
	}
	return BuildView(f), nil
}

// BuildView is View for an already loaded file.
func BuildView(f File) View {
	f = Normalize(f)
	st, enough := Trends(f.Snapshots)
	return View{Snapshots: f.Snapshots, Statements: st, Enough: enough}
}

// Clear deletes the whole history.
func (s *Service) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Store.Clear()
}
