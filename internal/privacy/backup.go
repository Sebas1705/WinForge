package privacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxBackups = 10

// Backup is one saved set of previous values, listed newest first. Tweaks are the ids it changed;
// Restored says the backup was already put back once.
type Backup struct {
	ID       string     `json:"id"`
	Time     time.Time  `json:"time"`
	Action   string     `json:"action"`
	Tweaks   []string   `json:"tweaks"`
	Restored *time.Time `json:"restored"`
}

// saved is a registry value as it was or as it was left: existence, type and raw data. The data is
// stored as base64 by encoding/json.
type saved struct {
	Exists bool   `json:"exists"`
	Type   uint32 `json:"type"`
	Data   []byte `json:"data"`
}

// backupEntry is one value that was changed: what it was (Before), what the change left (After, so
// a restore can tell that someone edited it since) and the keys the change had to create.
type backupEntry struct {
	Tweak   string   `json:"tweak"`
	Path    string   `json:"path"`
	Name    string   `json:"name"`
	Before  saved    `json:"before"`
	After   saved    `json:"after"`
	Created []string `json:"created,omitempty"`
}

type backupFile struct {
	ID         string        `json:"id"`
	Time       time.Time     `json:"time"`
	Action     string        `json:"action"`
	Entries    []backupEntry `json:"entries"`
	RestoredAt *time.Time    `json:"restoredAt,omitempty"`
}

var backupIDRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$`)

func readBackup(path string) (*backupFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bf backupFile
	if err := json.Unmarshal(b, &bf); err != nil {
		return nil, fmt.Errorf("backup %s is damaged: %w", filepath.Base(path), err)
	}
	return &bf, nil
}

// writeBackup saves bf under a new id built from its time, in a file that appears whole or not at all.
func writeBackup(dir string, bf *backupFile) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := bf.Time.UTC().Format("20060102T150405Z")
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, id+".json")); errors.Is(err, os.ErrNotExist) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	bf.ID = id
	return id, rewriteBackup(dir, id, bf)
}

func rewriteBackup(dir, id string, bf *backupFile) error {
	b, err := json.MarshalIndent(bf, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(dir, id+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func listBackups(dir string) ([]Backup, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Backup{}, nil
		}
		return nil, err
	}
	out := []Backup{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if e.IsDir() || !ok || !backupIDRe.MatchString(id) {
			continue
		}
		bf, err := readBackup(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // a damaged file is left alone and not offered
		}
		ids := []string{}
		for _, en := range bf.Entries {
			if !contains(ids, en.Tweak) {
				ids = append(ids, en.Tweak)
			}
		}
		out = append(out, Backup{ID: id, Time: bf.Time, Action: bf.Action, Tweaks: ids, Restored: bf.RestoredAt})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// pruneBackups keeps the newest few. Failing to delete an old one is not worth failing a change.
func pruneBackups(dir string) {
	list, err := listBackups(dir)
	if err != nil || len(list) <= maxBackups {
		return
	}
	for _, b := range list[maxBackups:] {
		_ = os.Remove(filepath.Join(dir, b.ID+".json"))
	}
}
