// Package drift answers "what changed on this PC since I applied this profile?". When a profile is
// applied, the catalog apps installed at that moment are remembered as a baseline; later, the
// baseline is compared with what is installed now. It only reads and remembers: nothing here
// installs, removes or edits anything.
package drift

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Baseline is the state of the PC when a profile was last applied or accepted.
type Baseline struct {
	At time.Time `json:"at"`
	// Apps maps every catalog app that was installed then to its version, "" when unknown.
	Apps map[string]string `json:"apps"`
}

// Change is an app whose version moved.
type Change struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Report is what differs between a baseline and now.
type Report struct {
	// Removed are the profile's apps that were installed at the baseline and no longer are.
	Removed []string `json:"removed"`
	// Added are apps installed now that were not at the baseline and are not part of the profile.
	Added []string `json:"added"`
	// Updated are the profile's apps whose version is not the one they had.
	Updated []Change `json:"updated"`
}

// Count is how many things differ.
func (r Report) Count() int { return len(r.Removed) + len(r.Added) + len(r.Updated) }

// Compare returns what changed. profile is the ids of the profile's apps; installed maps every catalog
// app installed now to its version. An app installed since the baseline that the profile already
// lists is not a change: it is the profile being completed. A version is compared only when both
// sides know it, because "unknown" is not a change. Results are sorted, so they do not shuffle.
func Compare(base Baseline, profile []string, installed map[string]string) Report {
	inProfile := make(map[string]bool, len(profile))
	for _, id := range profile {
		inProfile[id] = true
	}
	r := Report{Removed: []string{}, Added: []string{}, Updated: []Change{}}
	for id := range base.Apps {
		if !inProfile[id] {
			continue
		}
		now, ok := installed[id]
		if !ok {
			r.Removed = append(r.Removed, id)
			continue
		}
		if was := base.Apps[id]; was != "" && now != "" && was != now {
			r.Updated = append(r.Updated, Change{ID: id, From: was, To: now})
		}
	}
	for id := range installed {
		if _, was := base.Apps[id]; !was && !inProfile[id] {
			r.Added = append(r.Added, id)
		}
	}
	sort.Strings(r.Removed)
	sort.Strings(r.Added)
	sort.Slice(r.Updated, func(i, j int) bool { return r.Updated[i].ID < r.Updated[j].ID })
	return r
}

// Store keeps one baseline per profile id in a JSON file, written atomically.
type Store struct {
	Path string
	mu   sync.Mutex
}

// DefaultStore is %AppData%\WinForge\applied.json.
func DefaultStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(base, "WinForge", "applied.json")}, nil
}

type file struct {
	Profiles map[string]Baseline `json:"profiles"`
}

func (s *Store) load() (map[string]Baseline, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Baseline{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s is damaged (%v); fix or delete it", s.Path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Baseline{}
	}
	return f.Profiles, nil
}

func (s *Store) save(m map[string]Baseline) error {
	if len(m) == 0 {
		if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(file{Profiles: m}, "", "  ")
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

// All returns every baseline. A missing file is none; a damaged one is an error and is never
// overwritten, so a person's history is not lost to a bad write.
func (s *Store) All() (map[string]Baseline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Get returns the baseline of one profile.
func (s *Store) Get(id string) (Baseline, bool, error) {
	all, err := s.All()
	if err != nil {
		return Baseline{}, false, err
	}
	b, ok := all[id]
	return b, ok, nil
}

// Set remembers the state of the PC for a profile, replacing the earlier baseline.
func (s *Store) Set(id string, installed map[string]string, at time.Time) error {
	if id == "" {
		return errors.New("no profile given")
	}
	apps := make(map[string]string, len(installed))
	for k, v := range installed {
		apps[k] = v
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return err
	}
	all[id] = Baseline{At: at.UTC(), Apps: apps}
	return s.save(all)
}

// Remove forgets a profile's baseline. A profile that had none is not an error.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := all[id]; !ok {
		return nil
	}
	delete(all, id)
	return s.save(all)
}
