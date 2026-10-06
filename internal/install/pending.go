package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Pending is a run that did not finish: stopped, failed, or cut short by a
// restart. It lives on disk so the work can be picked up again - after the PC
// restarts, after relaunching as administrator, or after a crash.
type Pending struct {
	Title string `json:"title"`
	Steps []Step `json:"steps"`
}

// PendingStore keeps at most one Pending in a file.
type PendingStore struct{ Path string }

// DefaultPendingStore is %AppData%\WinForge\pending.json.
func DefaultPendingStore() (*PendingStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &PendingStore{Path: filepath.Join(base, "WinForge", "pending.json")}, nil
}

// Save replaces the stored run. An empty step list clears it.
func (s *PendingStore) Save(p Pending) error {
	if len(p.Steps) == 0 {
		return s.Clear()
	}
	b, err := json.MarshalIndent(p, "", "  ")
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

// Load returns the stored run, or nil when there is none or it is unreadable.
func (s *PendingStore) Load() *Pending {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return nil
	}
	var p Pending
	if json.Unmarshal(b, &p) != nil || len(p.Steps) == 0 {
		return nil
	}
	return &p
}

// Clear forgets the stored run.
func (s *PendingStore) Clear() error {
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Without returns the steps that are not in done.
func Without(steps []Step, done map[string]bool) []Step {
	out := []Step{}
	for _, s := range steps {
		if !done[string(s.Kind)+":"+s.ID] {
			out = append(out, s)
		}
	}
	return out
}

// StepKey identifies a step in a done set.
func StepKey(s Step) string { return string(s.Kind) + ":" + s.ID }
