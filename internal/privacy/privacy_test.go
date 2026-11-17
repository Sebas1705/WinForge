package privacy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mgr(f *fakeSys) *Manager {
	t0 := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	n := 0
	return &Manager{Sys: f, Now: func() time.Time { n++; return t0.Add(time.Duration(n) * time.Second) }}
}

func view(t *testing.T, m *Manager, id string) TweakView {
	t.Helper()
	l, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range l {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("tweak %s not listed", id)
	return TweakView{}
}

func TestTable(t *testing.T) {
	const cdmKey = `Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
	type row struct {
		path, name string
		private    uint32
		defaults   []uint32
	}
	want := map[string][]row{
		"advertising-id":       {{`Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`, "Enabled", 0, []uint32{1}}},
		"tailored-experiences": {{`Software\Microsoft\Windows\CurrentVersion\Privacy`, "TailoredExperiencesWithDiagnosticDataEnabled", 0, []uint32{1}}},
		"language-list":        {{`Control Panel\International\User Profile`, "HttpAcceptLanguageOptOut", 1, []uint32{0}}},
		"start-suggestions":    {{cdmKey, "SystemPaneSuggestionsEnabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-338388Enabled", 0, []uint32{1}}},
		"settings-suggestions": {{cdmKey, "SubscribedContent-338393Enabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-353694Enabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-353696Enabled", 0, []uint32{1}}},
		"tips-welcome":         {{cdmKey, "SoftLandingEnabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-338389Enabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-310093Enabled", 0, []uint32{1}}},
		"lockscreen-tips":      {{cdmKey, "RotatingLockScreenOverlayEnabled", 0, []uint32{1}}, {cdmKey, "SubscribedContent-338387Enabled", 0, []uint32{1}}},
		"start-most-used":      {{`Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "Start_TrackProgs", 0, []uint32{1}}},
		"start-recent-items":   {{`Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, "Start_TrackDocs", 0, []uint32{1}}},
		"start-web-search":     {{`Software\Policies\Microsoft\Windows\Explorer`, "DisableSearchBoxSuggestions", 1, []uint32{0}}},
		"feedback-prompts":     {{`Software\Microsoft\Siuf\Rules`, "NumberOfSIUFInPeriod", 0, nil}},
	}
	got := Tweaks()
	if len(got) != len(want) {
		t.Fatalf("%d tweaks, want %d", len(got), len(want))
	}
	owner := map[string]string{}
	for _, tw := range got {
		rows, ok := want[tw.ID]
		if !ok {
			t.Fatalf("unexpected tweak %s", tw.ID)
		}
		if len(tw.Changes) != len(rows) {
			t.Fatalf("%s: %d changes, want %d", tw.ID, len(tw.Changes), len(rows))
		}
		for i, c := range tw.Changes {
			r := rows[i]
			if c.Path != r.path || c.Name != r.name || c.Private != r.private || !reflect.DeepEqual(c.Defaults, r.defaults) {
				t.Errorf("%s[%d] = %+v, want %+v", tw.ID, i, c, r)
			}
			for _, d := range c.Defaults {
				if d == c.Private {
					t.Errorf("%s: the private value is also a default", tw.ID)
				}
			}
			k := strings.ToLower(c.Path + "|" + c.Name)
			if prev, dup := owner[k]; dup {
				t.Errorf("%s and %s both write %s", prev, tw.ID, k)
			}
			owner[k] = tw.ID
			if strings.HasPrefix(strings.ToUpper(c.Path), "HK") || strings.HasPrefix(c.Path, `\`) {
				t.Errorf("%s: path %q must be below HKEY_CURRENT_USER", tw.ID, c.Path)
			}
		}
		if tw.NameKey() != "privacy.t."+tw.ID+".name" || tw.WhyKey() != "privacy.t."+tw.ID+".why" {
			t.Errorf("%s: bad text keys", tw.ID)
		}
	}
}

func TestListStates(t *testing.T) {
	f := newFake(t)
	m := mgr(f)
	for _, tw := range m.mustList(t) {
		if tw.State != StateDefault {
			t.Errorf("%s on an empty registry is %s, want default", tw.ID, tw.State)
		}
		for _, c := range tw.Changes {
			if c.Existed || c.OldType != "" || c.Old != "" || c.Type != "REG_DWORD" || c.Error {
				t.Errorf("%s: bad preview %+v", tw.ID, c)
			}
			if !strings.HasPrefix(c.Path, `HKEY_CURRENT_USER\`) {
				t.Errorf("%s: path %q", tw.ID, c.Path)
			}
		}
	}

	f.putDWord(keyAdv, "Enabled", 0)
	if s := view(t, m, "advertising-id").State; s != StatePrivate {
		t.Errorf("Enabled=0: %s", s)
	}
	f.putDWord(keyAdv, "Enabled", 1)
	if s := view(t, m, "advertising-id").State; s != StateDefault {
		t.Errorf("Enabled=1: %s", s)
	}
	f.putDWord(keyPrivacy, "TailoredExperiencesWithDiagnosticDataEnabled", 2)
	if v := view(t, m, "tailored-experiences"); v.State != StateCustom || v.Changes[0].Old != "2" || v.Changes[0].OldType != "REG_DWORD" {
		t.Errorf("value 2: %+v", v)
	}
	f.str(keyExplorer, "Start_TrackProgs", "0") // a string "0" is not the DWORD Windows reads
	if v := view(t, m, "start-most-used"); v.State != StateCustom || v.Changes[0].OldType != "REG_SZ" || v.Changes[0].Old != "0" {
		t.Errorf("string value: %+v", v)
	}
	// A tweak with several values: half done is custom.
	f.putDWord(keyCDM, "SoftLandingEnabled", 0)
	if s := view(t, m, "tips-welcome").State; s != StateCustom {
		t.Errorf("partly done: %s", s)
	}
	f.putDWord(keyCDM, "SubscribedContent-338389Enabled", 0)
	f.putDWord(keyCDM, "SubscribedContent-310093Enabled", 0)
	if s := view(t, m, "tips-welcome").State; s != StatePrivate {
		t.Errorf("all done: %s", s)
	}
	f.failGet["Start_TrackDocs"] = errBoom
	if v := view(t, m, "start-recent-items"); v.State != StateUnknown || !v.Changes[0].Error {
		t.Errorf("unreadable: %+v", v)
	}
	if f.writeCount() != 0 {
		t.Errorf("listing wrote %d values", f.writeCount())
	}
}

func (m *Manager) mustList(t *testing.T) []TweakView {
	t.Helper()
	l, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// snapshot copies every value so a round trip can be compared bit for bit.
func snapshot(f *fakeSys) (map[string]Value, map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := map[string]Value{}
	for k, x := range f.vals {
		v[k] = Value{Exists: x.Exists, Type: x.Type, Data: append([]byte(nil), x.Data...)}
	}
	k := map[string]bool{}
	for p := range f.keys {
		k[p] = true
	}
	return v, k
}

// Every tweak, applied and restored, on a registry that has none of the values: the values and the
// keys it had to create are all gone again.
func TestApplyAndRestoreEveryTweakOnEmptyRegistry(t *testing.T) {
	f := newFake(t)
	f.addKey(`Software\Microsoft\Windows\CurrentVersion`) // exists on every Windows
	f.addKey(`Software\Microsoft`)
	f.addKey(`Control Panel\International`)
	m := mgr(f)
	v0, k0 := snapshot(f)

	var ids []string
	for _, tw := range Tweaks() {
		ids = append(ids, tw.ID)
	}
	res, err := m.Apply(ids)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != len(ids) || len(res.Failed) != 0 || res.BackupID == "" {
		t.Fatalf("result %+v", res)
	}
	for _, v := range m.mustList(t) {
		if v.State != StatePrivate {
			t.Errorf("%s is %s after Apply", v.ID, v.State)
		}
		for _, c := range v.Changes {
			if c.OldType != "REG_DWORD" {
				t.Errorf("%s: after Apply the value is %s", v.ID, c.OldType)
			}
		}
	}
	if f.broadcasts != 1 {
		t.Errorf("%d broadcasts, want 1", f.broadcasts)
	}
	rr, err := m.Restore(res.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rr.Restored) != len(ids) || len(rr.ChangedSince) != 0 || len(rr.Failed) != 0 {
		t.Fatalf("restore %+v", rr)
	}
	v1, k1 := snapshot(f)
	if !reflect.DeepEqual(v0, v1) {
		t.Errorf("values differ after restore: %v vs %v", v0, v1)
	}
	if !reflect.DeepEqual(k0, k1) {
		t.Errorf("keys differ after restore: %v vs %v", k0, k1)
	}
}

func TestRestorePreservesTypeAndExistingData(t *testing.T) {
	f := newFake(t)
	f.str(keyAdv, "Enabled", "1")                                             // a string where a number belongs
	f.putDWord(keyPrivacy, "TailoredExperiencesWithDiagnosticDataEnabled", 2) // a value we do not write
	f.put(keyExplorer, "Start_TrackProgs", TypeBinary, []byte{1, 2, 3})
	f.put(keyExplorer, "Start_TrackDocs", TypeQWord, []byte{1, 0, 0, 0, 0, 0, 0, 0})
	f.putDWord(keyLang, "HttpAcceptLanguageOptOut", 0)
	m := mgr(f)
	v0, k0 := snapshot(f)

	res, err := m.Apply([]string{"advertising-id", "tailored-experiences", "start-most-used", "start-recent-items", "language-list"})
	if err != nil || res.Applied != 5 {
		t.Fatalf("%+v %v", res, err)
	}
	got, _ := f.Get(keyAdv, "Enabled")
	if got.Type != TypeDWord || !reflect.DeepEqual(got.Data, dword(0)) {
		t.Errorf("applied value %+v", got)
	}
	if _, err := m.Restore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	v1, k1 := snapshot(f)
	if !reflect.DeepEqual(v0, v1) || !reflect.DeepEqual(k0, k1) {
		t.Errorf("not restored exactly:\n%v\n%v", v0, v1)
	}
	if got, _ := f.Get(keyAdv, "Enabled"); got.Type != TypeString || Display(got) != "1" {
		t.Errorf("type lost: %+v", got)
	}
}

func TestOnlyValuesThatChangeAreTouchedAndSavedFirst(t *testing.T) {
	f := newFake(t)
	f.putDWord(keyCDM, "SoftLandingEnabled", 0) // already private
	f.putDWord(keyCDM, "SubscribedContent-338389Enabled", 1)
	f.putDWord(keyCDM, "OtherValueOfAnApp", 7) // not ours
	m := mgr(f)
	res, err := m.Apply([]string{"tips-welcome"})
	if err != nil || res.Applied != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if got := strings.Join(f.writes, ","); got != "set SubscribedContent-338389Enabled,set SubscribedContent-310093Enabled" {
		t.Errorf("writes: %s", got)
	}
	if v, _ := f.Get(keyCDM, "OtherValueOfAnApp"); !reflect.DeepEqual(v.Data, dword(7)) {
		t.Error("a value that is not ours changed")
	}
	// The backup file exists before the values changed: it is whole JSON naming both values.
	b, err := os.ReadFile(filepath.Join(f.backupDir, res.BackupID+".json"))
	if err != nil || !strings.Contains(string(b), "SubscribedContent-338389Enabled") || strings.Contains(string(b), `"SoftLandingEnabled"`) {
		t.Errorf("backup content: %s %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(f.backupDir, res.BackupID+".json.tmp")); err == nil {
		t.Error("temporary backup file left behind")
	}
	// A second run finds nothing to do: nothing is written and no backup is made.
	n := f.writeCount()
	res2, err := m.Apply([]string{"tips-welcome", "tips-welcome"})
	if err != nil || res2.Applied != 0 || res2.BackupID != "" || !reflect.DeepEqual(res2.Skipped, []string{"tips-welcome"}) {
		t.Errorf("second run %+v %v", res2, err)
	}
	if f.writeCount() != n {
		t.Error("a second Apply wrote again")
	}
	if l, _ := m.Backups(); len(l) != 1 {
		t.Errorf("%d backups", len(l))
	}
}

func TestUnknownIDRefusesEverythingAndWritesNothing(t *testing.T) {
	f := newFake(t)
	m := mgr(f)
	for _, act := range []func([]string) (Result, error){m.Apply, m.Reset} {
		_, err := act([]string{"advertising-id", "format-disk"})
		if err != ErrUnknownTweak {
			t.Errorf("err = %v", err)
		}
	}
	if f.writeCount() != 0 {
		t.Error("something was written")
	}
	if _, err := os.Stat(f.backupDir); err == nil {
		if e, _ := os.ReadDir(f.backupDir); len(e) != 0 {
			t.Error("a backup was made")
		}
	}
}

func TestResetRemovesValuesAndCanBeUndone(t *testing.T) {
	f := newFake(t)
	f.putDWord(keyAdv, "Enabled", 0)
	f.putDWord(keyPolicy, "DisableSearchBoxSuggestions", 1)
	f.putDWord(keyExplorer, "Start_TrackProgs", 1) // already default: left alone
	m := mgr(f)
	v0, k0 := snapshot(f)
	res, err := m.Reset([]string{"advertising-id", "start-web-search", "start-most-used"})
	if err != nil || res.Applied != 2 || !reflect.DeepEqual(res.Skipped, []string{"start-most-used"}) {
		t.Fatalf("%+v %v", res, err)
	}
	if v, _ := f.Get(keyAdv, "Enabled"); v.Exists {
		t.Error("value still there")
	}
	if s := view(t, m, "advertising-id").State; s != StateDefault {
		t.Errorf("state %s", s)
	}
	if l, _ := m.Backups(); len(l) != 1 || l[0].Action != ActionReset {
		t.Errorf("backups %+v", l)
	}
	if _, err := m.Restore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	v1, k1 := snapshot(f)
	if !reflect.DeepEqual(v0, v1) || !reflect.DeepEqual(k0, k1) {
		t.Error("reset not undone exactly")
	}
}

func TestRestoreReportsValuesEditedSinceButRestoresThem(t *testing.T) {
	f := newFake(t)
	f.putDWord(keyAdv, "Enabled", 1)
	m := mgr(f)
	res, _ := m.Apply([]string{"advertising-id", "start-most-used"})
	f.putDWord(keyAdv, "Enabled", 5) // someone else changed it
	rr, err := m.Restore(res.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rr.ChangedSince, []string{"advertising-id"}) || len(rr.Restored) != 2 {
		t.Errorf("%+v", rr)
	}
	if v, _ := f.Get(keyAdv, "Enabled"); !reflect.DeepEqual(v.Data, dword(1)) {
		t.Error("not restored")
	}
	// Restoring the same backup again is allowed and still exact.
	if _, err := m.Restore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	l, _ := m.Backups()
	if len(l) != 1 || l[0].Restored == nil {
		t.Errorf("backup not marked restored: %+v", l)
	}
}

func TestPartialFailureKeepsWhatWasWrittenInTheBackup(t *testing.T) {
	f := newFake(t)
	f.failSet["SubscribedContent-338388Enabled"] = errBoom
	f.putDWord(keyCDM, "SystemPaneSuggestionsEnabled", 1)
	m := mgr(f)
	res, err := m.Apply([]string{"start-suggestions", "advertising-id"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 1 || !reflect.DeepEqual(res.Failed, []string{"start-suggestions"}) || res.BackupID == "" {
		t.Fatalf("%+v", res)
	}
	// The half-written tweak is still undoable.
	if _, err := m.Restore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Get(keyCDM, "SystemPaneSuggestionsEnabled"); !reflect.DeepEqual(v.Data, dword(1)) {
		t.Error("partly written value not restored")
	}
	// Everything fails: no backup is left.
	f2 := newFake(t)
	f2.failSet["Enabled"] = errBoom
	res, err = mgr(f2).Apply([]string{"advertising-id"})
	if err != nil || res.Applied != 0 || res.BackupID != "" || len(res.Failed) != 1 {
		t.Errorf("%+v %v", res, err)
	}
	if e, _ := os.ReadDir(f2.backupDir); len(e) != 0 {
		t.Errorf("%d files left", len(e))
	}
}

func TestUnreadableValueIsNeverWritten(t *testing.T) {
	f := newFake(t)
	f.failGet["Enabled"] = errBoom
	res, err := mgr(f).Apply([]string{"advertising-id", "start-most-used"})
	if err != nil || res.Applied != 1 || !reflect.DeepEqual(res.Failed, []string{"advertising-id"}) {
		t.Errorf("%+v %v", res, err)
	}
	for _, w := range f.writes {
		if w == "set Enabled" {
			t.Error("wrote a value it could not read")
		}
	}
}

func TestBackupsAreListedNewestFirstAndPrunedToTen(t *testing.T) {
	f := newFake(t)
	m := mgr(f)
	var ids []string
	for i := 0; i < 13; i++ {
		var res Result
		if i%2 == 0 {
			res, _ = m.Apply([]string{"advertising-id"})
		} else {
			res, _ = m.Reset([]string{"advertising-id"})
		}
		ids = append(ids, res.BackupID)
	}
	l, err := m.Backups()
	if err != nil || len(l) != 10 {
		t.Fatalf("%d backups, %v", len(l), err)
	}
	if l[0].ID != ids[12] || l[9].ID != ids[3] {
		t.Errorf("order: %s ... %s", l[0].ID, l[9].ID)
	}
	if _, err := m.Restore(ids[0]); err != ErrBackupNotFound {
		t.Errorf("pruned backup: %v", err)
	}
}

func TestSameSecondBackupsGetDistinctIDs(t *testing.T) {
	f := newFake(t)
	m := &Manager{Sys: f, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	a, _ := m.Apply([]string{"advertising-id"})
	b, _ := m.Reset([]string{"advertising-id"})
	if a.BackupID == b.BackupID || b.BackupID == "" {
		t.Errorf("%q %q", a.BackupID, b.BackupID)
	}
}

func TestDamagedAndForeignBackupFiles(t *testing.T) {
	f := newFake(t)
	m := mgr(f)
	good, _ := m.Apply([]string{"advertising-id"})

	// A truncated file: not offered, and restoring it is refused with a clear error, nothing written.
	bad := "20260101T000000Z"
	if err := os.WriteFile(filepath.Join(f.backupDir, bad+".json"), []byte(`{"id": "x", "entries": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(f.backupDir, "notes.txt"), []byte("hi"), 0o600)
	l, err := m.Backups()
	if err != nil || len(l) != 1 || l[0].ID != good.BackupID {
		t.Fatalf("%+v %v", l, err)
	}
	n := f.writeCount()
	if _, err := m.Restore(bad); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Errorf("restore of a damaged file: %v", err)
	}
	if f.writeCount() != n {
		t.Error("a damaged backup wrote")
	}
	// Ids that are not ids never reach the file system.
	for _, id := range []string{"", "../x", `..\..\evil`, "20260101T000000Z.json", "nope"} {
		if _, err := m.Restore(id); err != ErrBackupNotFound {
			t.Errorf("Restore(%q) = %v", id, err)
		}
	}
	if _, err := m.Restore("20990101T000000Z"); err != ErrBackupNotFound {
		t.Errorf("missing: %v", err)
	}
}

// A backup that was edited to name a value that is not one of the tweaks' own is not obeyed.
func TestRestoreIgnoresValuesOutsideTheTable(t *testing.T) {
	f := newFake(t)
	f.putDWord(`Software\Other`, "Precious", 9)
	m := mgr(f)
	bf := backupFile{ID: "20260101T000000Z", Time: time.Now().UTC(), Action: ActionApply, Entries: []backupEntry{
		{Tweak: "advertising-id", Path: `Software\Other`, Name: "Precious", Before: saved{}, After: saved{Exists: true, Type: TypeDWord, Data: dword(9)}},
		{Tweak: "advertising-id", Path: keyAdv, Name: "Enabled", Before: saved{Exists: true, Type: TypeDWord, Data: dword(1)}, After: saved{Exists: true, Type: TypeDWord, Data: dword(0)}, Created: []string{"Software", `Software\Other`}},
	}}
	bid, err := writeBackup(f.backupDir, &bf)
	if err != nil {
		t.Fatal(err)
	}
	rr, err := m.Restore(bid)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rr.Failed, []string{"advertising-id"}) {
		t.Errorf("%+v", rr)
	}
	if v, _ := f.Get(`Software\Other`, "Precious"); !v.Exists {
		t.Error("a value outside the table was deleted")
	}
	if ok, _ := f.KeyExists("Software"); !ok {
		t.Error("a top-level key was removed")
	}
	// The legitimate entry was still restored, and the backup is not marked as done.
	if v, _ := f.Get(keyAdv, "Enabled"); !reflect.DeepEqual(v.Data, dword(1)) {
		t.Error("the valid entry was not restored")
	}
	if l, _ := m.Backups(); len(l) != 1 || l[0].Restored != nil {
		t.Errorf("%+v", l)
	}
}

func TestDisplayAndTypeNames(t *testing.T) {
	f := newFake(t)
	f.str("a", "s", "héllo")
	s, _ := f.Get("a", "s")
	if Display(s) != "héllo" || TypeName(TypeString) != "REG_SZ" || TypeName(99) != "REG_99" {
		t.Error("display")
	}
	if Display(Value{Type: TypeBinary, Data: []byte{0xde, 0xad}}) != "dead" || Display(Value{Type: TypeQWord, Data: []byte{2, 0, 0, 0, 0, 0, 0, 0}}) != "2" {
		t.Error("display binary/qword")
	}
}
