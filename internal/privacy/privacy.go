package privacy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

// The state of a tweak, read from the registry.
const (
	StatePrivate = "private" // every value is at the private setting
	StateDefault = "default" // every value is absent or at Windows' own setting
	StateCustom  = "custom"  // anything else: a mix, or a value this tweak does not write
	StateUnknown = "unknown" // the registry could not be read
)

// What a backup was made by.
const (
	ActionApply = "apply"
	ActionReset = "reset"
)

// ErrUnknownTweak is returned when a request names a tweak that is not in the curated list.
var ErrUnknownTweak = errors.New("that setting is not in the list")

// ChangeView is one registry value of a tweak, as the page previews it: where it is, what it holds
// now, and what applying writes. Applying always writes a REG_DWORD; resetting removes the value.
type ChangeView struct {
	Path    string `json:"path"` // starts with HKEY_CURRENT_USER
	Name    string `json:"name"`
	Existed bool   `json:"existed"`
	OldType string `json:"oldType"` // e.g. REG_DWORD, "" when the value does not exist
	Old     string `json:"old"`     // the stored data as text, "" when the value does not exist
	Type    string `json:"type"`    // the type applying writes
	Private uint32 `json:"private"` // the data applying writes
	Error   bool   `json:"error"`   // the value could not be read
}

// TweakView is one tweak with its current state. The text is the interface's: NameKey and WhyKey
// are keys, never prose.
type TweakView struct {
	ID      string       `json:"id"`
	Area    string       `json:"area"`
	NameKey string       `json:"nameKey"`
	WhyKey  string       `json:"whyKey"`
	State   string       `json:"state"`
	Changes []ChangeView `json:"changes"`
}

// Result is what Apply or Reset did. Applied counts the tweaks changed; Skipped lists the ones that
// were already in the asked state; Failed lists the ones that could not be changed (fully or in
// part: whatever was written is in the backup). BackupID is "" when nothing was written.
type Result struct {
	Applied  int      `json:"applied"`
	Skipped  []string `json:"skipped"`
	Failed   []string `json:"failed"`
	BackupID string   `json:"backupId"`
}

// Manager reads and changes the tweaks. Now may be nil (the real clock).
type Manager struct {
	Sys System
	Now func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// List reads every tweak. A value that cannot be read makes its tweak "unknown"; nothing is written.
func (m *Manager) List() ([]TweakView, error) {
	out := make([]TweakView, 0, len(tweaks))
	for _, t := range tweaks {
		v := TweakView{ID: t.ID, Area: t.Area, NameKey: t.NameKey(), WhyKey: t.WhyKey(), State: StateDefault, Changes: make([]ChangeView, 0, len(t.Changes))}
		allPrivate, allDefault, unknown := true, true, false
		for _, c := range t.Changes {
			cv := ChangeView{Path: "HKEY_CURRENT_USER\\" + c.Path, Name: c.Name, Type: "REG_DWORD", Private: c.Private}
			cur, err := m.Sys.Get(c.Path, c.Name)
			if err != nil {
				cv.Error = true
				unknown = true
			} else {
				cv.Existed = cur.Exists
				if cur.Exists {
					cv.OldType = TypeName(cur.Type)
					cv.Old = Display(cur)
				}
				if !isPrivate(c, cur) {
					allPrivate = false
				}
				if !isDefault(c, cur) {
					allDefault = false
				}
			}
			v.Changes = append(v.Changes, cv)
		}
		switch {
		case unknown:
			v.State = StateUnknown
		case allPrivate:
			v.State = StatePrivate
		case allDefault:
			v.State = StateDefault
		default:
			v.State = StateCustom
		}
		out = append(out, v)
	}
	return out, nil
}

// Apply writes the private setting of the chosen tweaks. Values that already hold it are left
// alone. Before the first write the previous existence, type and data of every value that will
// change is saved in a backup. An id that is not a tweak refuses the whole request.
func (m *Manager) Apply(ids []string) (Result, error) { return m.change(ids, ActionApply) }

// Reset removes the values of the chosen tweaks so Windows decides again, the same way, with a backup.
func (m *Manager) Reset(ids []string) (Result, error) { return m.change(ids, ActionReset) }

type plannedEntry struct {
	tweak string
	entry backupEntry
}

// plan works out what Apply or Reset would write for the chosen tweaks, reading the registry only.
// An id that is not a tweak refuses the whole request. res carries Skipped (already as asked) and
// Failed (a value could not be read); chosen holds the tweaks asked for and planned the ones that
// will be changed.
func (m *Manager) plan(ids []string, action string) (chosen []Tweak, plan []plannedEntry, planned map[string]bool, res Result, err error) {
	res = Result{Skipped: []string{}, Failed: []string{}}
	seen := map[string]bool{}
	for _, id := range ids {
		t, ok := tweakByID(id)
		if !ok {
			return nil, nil, nil, res, ErrUnknownTweak
		}
		if !seen[id] {
			seen[id] = true
			chosen = append(chosen, t)
		}
	}

	planned = map[string]bool{}
	for _, t := range chosen {
		var entries []plannedEntry
		failed := false
		for _, c := range t.Changes {
			cur, err := m.Sys.Get(c.Path, c.Name)
			if err != nil {
				failed = true
				break
			}
			var after saved
			if action == ActionApply {
				if isPrivate(c, cur) {
					continue
				}
				after = saved{Exists: true, Type: TypeDWord, Data: dword(c.Private)}
			} else if !cur.Exists || isDefault(c, cur) {
				continue
			}
			e := backupEntry{Tweak: t.ID, Path: c.Path, Name: c.Name, Before: saved{Exists: cur.Exists, Type: cur.Type, Data: cur.Data}, After: after}
			if action == ActionApply {
				e.Created = m.missingKeys(c.Path)
			}
			entries = append(entries, plannedEntry{t.ID, e})
		}
		switch {
		case failed:
			res.Failed = append(res.Failed, t.ID)
		case len(entries) == 0:
			res.Skipped = append(res.Skipped, t.ID)
		default:
			plan = append(plan, entries...)
			planned[t.ID] = true
		}
	}
	return chosen, plan, planned, res, nil
}

// PlannedChange is one registry value Apply would write or Reset would delete (Remove true).
type PlannedChange struct {
	Tweak   string `json:"tweak"`
	Path    string `json:"path"` // starts with HKEY_CURRENT_USER
	Name    string `json:"name"`
	Existed bool   `json:"existed"`
	OldType string `json:"oldType"` // "" when the value does not exist
	Old     string `json:"old"`
	Remove  bool   `json:"remove"`
	NewType string `json:"newType"` // "" when the value is removed
	New     string `json:"new"`
}

// Preview says exactly what Apply (or Reset, with action ActionReset) would change for the chosen
// ids, without writing anything: the same plan, read from the registry now. Skipped lists the ids
// already as asked and Failed the ones whose values cannot be read.
func (m *Manager) Preview(ids []string, action string) ([]PlannedChange, Result, error) {
	if action != ActionApply && action != ActionReset {
		return nil, Result{Skipped: []string{}, Failed: []string{}}, errors.New("unknown action")
	}
	_, plan, _, res, err := m.plan(ids, action)
	if err != nil {
		return nil, res, err
	}
	out := make([]PlannedChange, 0, len(plan))
	for _, p := range plan {
		e := p.entry
		pc := PlannedChange{Tweak: e.Tweak, Path: "HKEY_CURRENT_USER\\" + e.Path, Name: e.Name, Existed: e.Before.Exists, Remove: action == ActionReset}
		if e.Before.Exists {
			pc.OldType = TypeName(e.Before.Type)
			pc.Old = Display(Value{Exists: true, Type: e.Before.Type, Data: e.Before.Data})
		}
		if action == ActionApply {
			pc.NewType = TypeName(e.After.Type)
			pc.New = Display(Value{Exists: true, Type: e.After.Type, Data: e.After.Data})
		}
		out = append(out, pc)
	}
	return out, res, nil
}

func (m *Manager) change(ids []string, action string) (Result, error) {
	chosen, plan, planned, res, err := m.plan(ids, action)
	if err != nil {
		return res, err
	}
	if len(plan) == 0 {
		return res, nil
	}

	dir, err := m.Sys.BackupDir()
	if err != nil {
		return res, fmt.Errorf("no place to save a backup: %w", err)
	}
	bf := backupFile{Time: m.now().UTC(), Action: action}
	for _, p := range plan {
		bf.Entries = append(bf.Entries, p.entry)
	}
	id, err := writeBackup(dir, &bf)
	if err != nil {
		return res, fmt.Errorf("could not save a backup, nothing was changed: %w", err)
	}

	var done []backupEntry
	bad := map[string]bool{}
	for _, p := range plan {
		var werr error
		if action == ActionApply {
			werr = m.Sys.Set(p.entry.Path, p.entry.Name, p.entry.After.Type, p.entry.After.Data)
		} else {
			werr = m.Sys.Delete(p.entry.Path, p.entry.Name)
		}
		if werr != nil {
			bad[p.tweak] = true
			continue
		}
		done = append(done, p.entry)
	}
	for _, t := range chosen {
		if !planned[t.ID] {
			continue
		}
		if bad[t.ID] {
			res.Failed = append(res.Failed, t.ID)
		} else {
			res.Applied++
		}
	}
	switch {
	case len(done) == 0:
		_ = os.Remove(filepath.Join(dir, id+".json"))
		return res, nil
	case len(done) < len(bf.Entries):
		bf.Entries = done
		if err := rewriteBackup(dir, id, &bf); err != nil {
			return res, fmt.Errorf("saved backup could not be trimmed: %w", err)
		}
	}
	res.BackupID = id
	_ = m.Sys.Broadcast()
	pruneBackups(dir)
	return res, nil
}

// missingKeys lists the keys on the way to path that do not exist yet, deepest first, so an undo
// can remove the ones this change created (only when they are empty again).
func (m *Manager) missingKeys(path string) []string {
	var out []string
	for p := path; p != ""; {
		ok, err := m.Sys.KeyExists(p)
		if err != nil || ok {
			break
		}
		out = append(out, p)
		i := strings.LastIndex(p, `\`)
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return out
}

// Backups lists the saved backups, newest first. A folder that does not exist yet has none.
func (m *Manager) Backups() ([]Backup, error) {
	dir, err := m.Sys.BackupDir()
	if err != nil {
		return nil, err
	}
	return listBackups(dir)
}

// RestoreResult says what Restore did, by tweak id. ChangedSince names the ones whose values were
// different from what the change left (someone edited them since): they are restored anyway.
type RestoreResult struct {
	Restored     []string `json:"restored"`
	ChangedSince []string `json:"changedSince"`
	Failed       []string `json:"failed"`
}

// ErrBackupNotFound is returned for an id that names no backup.
var ErrBackupNotFound = errors.New("that backup does not exist")

// Restore puts back exactly what a backup holds, for the values that backup changed and no others:
// a value that did not exist is removed again (and a key the change created is removed if it is
// empty), and one that existed gets its original type and data back. Values edited since the
// change are restored too and named in ChangedSince. The backup is marked as restored.
func (m *Manager) Restore(id string) (RestoreResult, error) {
	out := RestoreResult{Restored: []string{}, ChangedSince: []string{}, Failed: []string{}}
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
	restored, changed, failed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := len(bf.Entries) - 1; i >= 0; i-- {
		e := bf.Entries[i]
		if !known(e.Path, e.Name) {
			failed[e.Tweak] = true
			continue
		}
		cur, gerr := m.Sys.Get(e.Path, e.Name)
		if gerr != nil || !sameValue(cur, e.After) {
			changed[e.Tweak] = true
		}
		var werr error
		if e.Before.Exists {
			werr = m.Sys.Set(e.Path, e.Name, e.Before.Type, e.Before.Data)
		} else {
			werr = m.Sys.Delete(e.Path, e.Name)
			if werr == nil {
				for _, k := range e.Created {
					// Only the key of the value or one above it, as the backup recorded: never a key
					// a damaged file names from somewhere else.
					if !ownKey(k, e.Path) {
						continue
					}
					_ = m.Sys.DeleteKeyIfEmpty(k)
				}
			}
		}
		if werr != nil {
			failed[e.Tweak] = true
			continue
		}
		restored[e.Tweak] = true
	}
	for _, e := range bf.Entries {
		if restored[e.Tweak] && !failed[e.Tweak] && !contains(out.Restored, e.Tweak) {
			out.Restored = append(out.Restored, e.Tweak)
		}
		if changed[e.Tweak] && !contains(out.ChangedSince, e.Tweak) {
			out.ChangedSince = append(out.ChangedSince, e.Tweak)
		}
		if failed[e.Tweak] && !contains(out.Failed, e.Tweak) {
			out.Failed = append(out.Failed, e.Tweak)
		}
	}
	if len(restored) > 0 {
		_ = m.Sys.Broadcast()
	}
	if len(failed) == 0 {
		t := m.now().UTC()
		bf.RestoredAt = &t
		_ = rewriteBackup(dir, id, bf)
	}
	return out, nil
}

// ownKey reports whether key is path itself or one of its parents, at least two levels deep (a
// top-level key such as Software is never removed).
func ownKey(key, path string) bool {
	k, p := strings.ToLower(key), strings.ToLower(path)
	return strings.Contains(k, `\`) && (k == p || strings.HasPrefix(p, k+`\`))
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func sameValue(cur Value, s saved) bool {
	if cur.Exists != s.Exists {
		return false
	}
	return !cur.Exists || cur.Type == s.Type && bytes.Equal(cur.Data, s.Data)
}

func dword(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// asDWord reads a value as a number when it is a REG_DWORD.
func asDWord(v Value) (uint32, bool) {
	if !v.Exists || v.Type != TypeDWord || len(v.Data) != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(v.Data), true
}

func isPrivate(c Change, v Value) bool {
	n, ok := asDWord(v)
	return ok && n == c.Private
}

func isDefault(c Change, v Value) bool {
	if !v.Exists {
		return true
	}
	n, ok := asDWord(v)
	if !ok {
		return false
	}
	for _, d := range c.Defaults {
		if n == d {
			return true
		}
	}
	return false
}

// TypeName is the registry's own name for a value type.
func TypeName(t uint32) string {
	switch t {
	case TypeNone:
		return "REG_NONE"
	case TypeString:
		return "REG_SZ"
	case TypeExpand:
		return "REG_EXPAND_SZ"
	case TypeBinary:
		return "REG_BINARY"
	case TypeDWord:
		return "REG_DWORD"
	case TypeMultiSZ:
		return "REG_MULTI_SZ"
	case TypeQWord:
		return "REG_QWORD"
	}
	return fmt.Sprintf("REG_%d", t)
}

// Display is a stored value as text: a number for DWORD and QWORD, the text for strings, and the
// bytes in hexadecimal for anything else.
func Display(v Value) string {
	switch v.Type {
	case TypeDWord:
		if len(v.Data) == 4 {
			return fmt.Sprint(binary.LittleEndian.Uint32(v.Data))
		}
	case TypeQWord:
		if len(v.Data) == 8 {
			return fmt.Sprint(binary.LittleEndian.Uint64(v.Data))
		}
	case TypeString, TypeExpand:
		if len(v.Data)%2 == 0 {
			u := make([]uint16, len(v.Data)/2)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(v.Data[2*i:])
			}
			return strings.TrimRight(string(utf16.Decode(u)), "\x00")
		}
	}
	return fmt.Sprintf("%x", v.Data)
}
