package envdoctor

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

// FixResult is what Fix did: how many of the chosen findings were fixed, the ids that could not
// be (they no longer match the machine, or the registry refused), and the backup that holds what
// was replaced ("" when nothing was written).
type FixResult struct {
	Applied  int      `json:"applied"`
	Failed   []string `json:"failed"`
	BackupID string   `json:"backupId"`
}

// Backup is one saved set of previous values, listed newest first.
type Backup struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Changed []string  `json:"changed"` // the variables the fix changed
}

// RestoreResult says what Restore wrote. ChangedSince lists variables that had a different value
// than the fix left, which were restored anyway.
type RestoreResult struct {
	Restored     []string
	ChangedSince []string
}

// ErrBackupNotFound is returned for an id that names no backup.
var ErrBackupNotFound = errors.New("that backup does not exist")

// backupFile is what is stored: for each variable the value and type before the fix, and the value
// the fix wrote (or that it removed the variable), so Restore can tell when it changed since.
type backupFile struct {
	ID   string      `json:"id"`
	Time time.Time   `json:"time"`
	Vars []backupVar `json:"vars"`
}

type backupVar struct {
	Name    string `json:"name"`
	Value   string `json:"value"` // before the fix
	Type    uint32 `json:"type"`  // before the fix
	Deleted bool   `json:"deleted,omitempty"`
	After   string `json:"after,omitempty"`
}

var backupIDRe = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$`)

// plannedChange is one user variable a fix will rewrite or remove.
type plannedChange struct {
	name    string
	old     Raw
	remove  map[int]string // PATH segments to drop: index -> the segment text the finding named
	deleted bool
	ids     []string
	newVal  string
}

// Fix applies the chosen fixable findings to the user's variables. The scan is repeated first, so
// an id that no longer matches what is on the machine is reported in Failed and nothing is written
// for it. Before the first write, the previous value and type of every variable that will change
// is saved in a backup. PATH entries are removed by position, and every other segment stays exactly
// as it was written, %VAR% forms included.
func (m *Manager) Fix(ids []string) (FixResult, error) {
	res := FixResult{Failed: []string{}}
	st, err := m.scan()
	if err != nil {
		return res, err
	}
	byID := make(map[string]Finding, len(st.report.Findings))
	for _, f := range st.report.Findings {
		byID[f.ID] = f
	}

	plans := map[string]*plannedChange{}
	var order []string
	plan := func(name string) *plannedChange {
		if p, ok := plans[name]; ok {
			return p
		}
		old, ok := st.userEnv[name]
		if !ok {
			return nil
		}
		p := &plannedChange{name: name, old: old, remove: map[int]string{}}
		plans[name] = p
		order = append(order, name)
		return p
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		f, ok := byID[id]
		if !ok || !f.Fixable || f.Scope != User {
			res.Failed = append(res.Failed, id)
			continue
		}
		p := plan(f.variable)
		if p == nil {
			res.Failed = append(res.Failed, id)
			continue
		}
		switch f.FixKind {
		case FixRemovePathEntry:
			p.remove[f.index] = f.Subject
		case FixUnsetVariable:
			p.deleted = true
		default:
			res.Failed = append(res.Failed, id)
			continue
		}
		p.ids = append(p.ids, id)
	}

	var active []*plannedChange
	for _, name := range order {
		p := plans[name]
		if !p.deleted {
			segs := splitPath(p.old.Value)
			ok := true
			for idx, subject := range p.remove {
				if idx < 0 || idx >= len(segs) || segs[idx] != subject {
					ok = false
				}
			}
			if !ok {
				res.Failed = append(res.Failed, p.ids...)
				continue
			}
			kept := make([]string, 0, len(segs))
			for i, s := range segs {
				if _, drop := p.remove[i]; !drop {
					kept = append(kept, s)
				}
			}
			p.newVal = strings.Join(kept, ";")
		}
		active = append(active, p)
	}
	if len(active) == 0 {
		return res, nil
	}

	dir, err := m.Sys.BackupDir()
	if err != nil {
		return res, fmt.Errorf("no place to save a backup: %w", err)
	}
	now := m.now().UTC()
	bf := backupFile{Time: now}
	for _, p := range active {
		bf.Vars = append(bf.Vars, backupVar{Name: p.name, Value: p.old.Value, Type: p.old.Type, Deleted: p.deleted, After: p.newVal})
	}
	id, err := writeBackup(dir, &bf)
	if err != nil {
		return res, fmt.Errorf("could not save a backup, nothing was changed: %w", err)
	}

	var done []backupVar
	for i, p := range active {
		var werr error
		if p.deleted {
			werr = m.Sys.DeleteUserVar(p.name)
		} else {
			werr = m.Sys.SetUserVar(p.name, p.newVal, p.old.Type)
		}
		if werr != nil {
			res.Failed = append(res.Failed, p.ids...)
			continue
		}
		res.Applied += len(p.ids)
		done = append(done, bf.Vars[i])
	}
	switch {
	case len(done) == 0:
		_ = os.Remove(filepath.Join(dir, id+".json"))
		return res, nil
	case len(done) < len(bf.Vars):
		bf.Vars = done
		if err := rewriteBackup(dir, id, &bf); err != nil {
			return res, fmt.Errorf("saved backup could not be trimmed: %w", err)
		}
	}
	res.BackupID = id
	_ = m.Sys.Broadcast()
	pruneBackups(dir)
	return res, nil
}

// Backups lists the saved backups, newest first. A folder that does not exist yet has none.
func (m *Manager) Backups() ([]Backup, error) {
	dir, err := m.Sys.BackupDir()
	if err != nil {
		return nil, err
	}
	return listBackups(dir)
}

// Restore writes back the values a backup holds, for the variables that backup changed and no
// others, and tells running programs. A variable whose value is not what the fix left is restored
// anyway and named in ChangedSince. The id must be one Backups lists.
func (m *Manager) Restore(id string) (RestoreResult, error) {
	out := RestoreResult{Restored: []string{}, ChangedSince: []string{}}
	if !backupIDRe.MatchString(id) {
		return out, ErrBackupNotFound
	}
	dir, err := m.Sys.BackupDir()
	if err != nil {
		return out, err
	}
	bf, err := readBackup(filepath.Join(dir, id+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, ErrBackupNotFound
		}
		return out, err
	}
	cur, err := m.Sys.Env(User)
	if err != nil {
		return out, err
	}
	var errs []error
	for _, v := range bf.Vars {
		now, exists := cur[v.Name]
		if v.Deleted && exists || !v.Deleted && (!exists || now.Value != v.After) {
			out.ChangedSince = append(out.ChangedSince, v.Name)
		}
		if err := m.Sys.SetUserVar(v.Name, v.Value, v.Type); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", v.Name, err))
			continue
		}
		out.Restored = append(out.Restored, v.Name)
	}
	if len(out.Restored) > 0 {
		_ = m.Sys.Broadcast()
	}
	return out, errors.Join(errs...)
}

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
		changed := make([]string, 0, len(bf.Vars))
		for _, v := range bf.Vars {
			changed = append(changed, v.Name)
		}
		out = append(out, Backup{ID: id, Time: bf.Time, Changed: changed})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

// pruneBackups keeps the newest few. Failing to delete an old one is not worth failing a fix.
func pruneBackups(dir string) {
	list, err := listBackups(dir)
	if err != nil || len(list) <= maxBackups {
		return
	}
	for _, b := range list[maxBackups:] {
		_ = os.Remove(filepath.Join(dir, b.ID+".json"))
	}
}
