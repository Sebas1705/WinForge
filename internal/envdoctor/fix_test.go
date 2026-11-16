package envdoctor

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// messy is a user PATH with every kind of fixable entry plus entries that must survive untouched.
const messyPath = `C:\keep;C:\gone;%USERPROFILE%\bin;"C:\Program Files\Tool";;c:\KEEP\;C:\keep;C:\afile;rel`

func messyFake(t *testing.T) *fakeSys {
	f := newFake(t).dir(`C:\keep`, `C:\Users\me\bin`, `C:\Program Files\Tool`).file(`C:\afile`)
	f.setUserX("Path", messyPath)
	f.setUser("JAVA_HOME", `C:\jdk-gone`)
	f.setUser("Other", `untouched`)
	f.setMachine("Path", `C:\keep;C:\machine-gone`)
	return f
}

func ok(t *testing.T, r Report, id string) string {
	t.Helper()
	if !mustFind(t, r, id).Fixable {
		t.Fatalf("%s must be fixable", id)
	}
	return id
}

func TestFixRemovesOnlyTheNamedSegmentsAndKeepsTheRestExactly(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	r, _ := m.Scan()
	res, err := m.Fix([]string{
		ok(t, r, `path-missing|user|C:\gone`),
		ok(t, r, `path-empty|user|`),
		ok(t, r, `path-duplicate|user|c:\KEEP\`),
		ok(t, r, `path-duplicate|user|C:\keep#2`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 4 || len(res.Failed) != 0 || res.BackupID == "" {
		t.Fatalf("%+v", res)
	}
	want := `C:\keep;%USERPROFILE%\bin;"C:\Program Files\Tool";C:\afile;rel`
	if got := f.user["Path"]; got.Value != want || got.Type != TypeExpand {
		t.Errorf("PATH = %+v, want %q as REG_EXPAND_SZ", got, want)
	}
	if f.user["JAVA_HOME"].Value != `C:\jdk-gone` || f.user["Other"].Value != "untouched" {
		t.Error("other variables are untouched")
	}
	if !reflect.DeepEqual(f.writes, []string{"set Path"}) || f.broadcasts != 1 {
		t.Errorf("writes %v, broadcasts %d", f.writes, f.broadcasts)
	}
	if f.machine["Path"].Value != `C:\keep;C:\machine-gone` {
		t.Error("machine is never written")
	}
}

func TestFixPreservesTheRegistryType(t *testing.T) {
	for _, typ := range []uint32{TypeString, TypeExpand} {
		f := newFake(t).dir(`C:\a`)
		f.user["Path"] = Raw{Value: `C:\a;C:\gone;%X%`, Type: typ}
		m := mgr(f)
		r, _ := m.Scan()
		if _, err := m.Fix([]string{`path-missing|user|C:\gone`}); err != nil {
			t.Fatal(err)
		}
		if got := f.user["Path"]; got.Type != typ || got.Value != `C:\a;%X%` {
			t.Errorf("type %d: %+v", typ, got)
		}
		_ = r
	}
}

func TestFixRemovesTheLastSegmentWithoutLeavingASeparatorBehind(t *testing.T) {
	cases := map[string]string{
		`C:\a;C:\gone`:          `C:\a`,
		`C:\gone;C:\a`:          `C:\a`,
		`C:\gone`:               ``,
		`C:\a;C:\gone;`:         `C:\a;`, // the trailing empty one stays: it was not named
		`C:\gone;;C:\a`:         `;C:\a`,
		`"C:\g;x";C:\gone;C:\a`: `"C:\g;x";C:\a`,
	}
	for in, want := range cases {
		f := newFake(t).dir(`C:\a`)
		f.setUser("Path", in)
		m := mgr(f)
		res, err := m.Fix([]string{`path-missing|user|C:\gone`})
		if err != nil || res.Applied != 1 {
			t.Fatalf("%q: %+v %v", in, res, err)
		}
		if got := f.user["Path"].Value; got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestFixRemovingEveryEmptySegment(t *testing.T) {
	f := newFake(t).dir(`C:\a`)
	f.setUser("Path", `;C:\a;;`)
	m := mgr(f)
	r, _ := m.Scan()
	var emptyIDs []string
	for _, fd := range r.Findings {
		if fd.Kind == KindPathEmpty {
			emptyIDs = append(emptyIDs, fd.ID)
		}
	}
	if len(emptyIDs) != 3 {
		t.Fatalf("%v", ids(r))
	}
	if res, err := m.Fix(emptyIDs); err != nil || res.Applied != 3 {
		t.Fatalf("%+v %v", res, err)
	}
	if got := f.user["Path"].Value; got != `C:\a` {
		t.Errorf("%q", got)
	}
}

func TestFixUnsetsAHomeAndBacksItUp(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, err := m.Fix([]string{`home-missing|user|JAVA_HOME`})
	if err != nil || res.Applied != 1 || res.BackupID == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, still := f.user["JAVA_HOME"]; still {
		t.Error("variable removed")
	}
	if !reflect.DeepEqual(f.writes, []string{"del JAVA_HOME"}) {
		t.Errorf("%v", f.writes)
	}
	if f.user["Path"].Value != messyPath {
		t.Error("PATH untouched when only a home is unset")
	}
}

func TestBackupRoundTrip(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	before := map[string]Raw{}
	for k, v := range f.user {
		before[k] = v
	}
	res, err := m.Fix([]string{`path-missing|user|C:\gone`, `home-missing|user|JAVA_HOME`})
	if err != nil || res.Applied != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	list, err := m.Backups()
	if err != nil || len(list) != 1 || list[0].ID != res.BackupID {
		t.Fatalf("%+v %v", list, err)
	}
	if !reflect.DeepEqual(list[0].Changed, []string{"Path", "JAVA_HOME"}) || list[0].Time.IsZero() {
		t.Errorf("%+v", list[0])
	}
	rr, err := m.Restore(res.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rr.Restored, []string{"Path", "JAVA_HOME"}) || len(rr.ChangedSince) != 0 {
		t.Errorf("%+v", rr)
	}
	if !reflect.DeepEqual(f.user, before) {
		t.Errorf("after restore:\n%+v\nbefore:\n%+v", f.user, before)
	}
	if f.broadcasts != 2 {
		t.Errorf("fix and restore each tell the system: %d", f.broadcasts)
	}
	if _, err := os.Stat(filepath.Join(f.backupDir, res.BackupID+".json")); err != nil {
		t.Errorf("a backup stays after being used: %v", err)
	}
}

func TestBackupIsWrittenBeforeAnyVariable(t *testing.T) {
	f := messyFake(t)
	var sawBackup bool
	g := &spySys{fakeSys: f, onWrite: func() {
		es, _ := os.ReadDir(f.backupDir)
		sawBackup = len(es) == 1
	}}
	m := &Manager{Sys: g, Now: func() time.Time { return epoch }}
	if _, err := m.Fix([]string{`path-missing|user|C:\gone`}); err != nil {
		t.Fatal(err)
	}
	if !sawBackup {
		t.Error("the backup must exist before the first write")
	}
}

type spySys struct {
	*fakeSys
	onWrite func()
}

func (s *spySys) SetUserVar(n, v string, typ uint32) error {
	s.onWrite()
	return s.fakeSys.SetUserVar(n, v, typ)
}

func TestBackupFileHoldsOldValuesAndTypes(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, _ := m.Fix([]string{`path-missing|user|C:\gone`})
	b, err := os.ReadFile(filepath.Join(f.backupDir, res.BackupID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{`"id"`, `"time"`, `"name": "Path"`, `"type": 2`, `%USERPROFILE%`} {
		if !strings.Contains(s, want) {
			t.Errorf("backup lacks %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Other") || strings.Contains(s, "JAVA_HOME") {
		t.Errorf("only the variables that changed are saved:\n%s", s)
	}
}

func TestRestoreReportsVariablesChangedSinceButRestoresThem(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, _ := m.Fix([]string{`path-missing|user|C:\gone`, `home-missing|user|JAVA_HOME`})
	// Someone edits PATH and sets JAVA_HOME again after the fix.
	f.user["Path"] = Raw{Value: `C:\somebody-else`, Type: TypeString}
	f.user["JAVA_HOME"] = Raw{Value: `C:\new`, Type: TypeString}
	rr, err := m.Restore(res.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rr.ChangedSince, []string{"Path", "JAVA_HOME"}) || len(rr.Restored) != 2 {
		t.Errorf("%+v", rr)
	}
	if f.user["Path"].Value != messyPath || f.user["Path"].Type != TypeExpand || f.user["JAVA_HOME"].Value != `C:\jdk-gone` {
		t.Errorf("restored anyway: %+v", f.user)
	}
}

func TestRestoreOnlyTouchesVariablesTheBackupChanged(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, _ := m.Fix([]string{`path-missing|user|C:\gone`})
	f.user["Other"] = Raw{Value: "edited", Type: TypeString}
	f.user["Extra"] = Raw{Value: "new", Type: TypeString}
	f.writes = nil
	if _, err := m.Restore(res.BackupID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.writes, []string{"set Path"}) || f.user["Other"].Value != "edited" || f.user["Extra"].Value != "new" {
		t.Errorf("%v %+v", f.writes, f.user)
	}
}

func TestRestoreRefusesBadIDs(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, _ := m.Fix([]string{`path-missing|user|C:\gone`})
	// A real file outside the pattern must not be readable through the id.
	outside := filepath.Join(filepath.Dir(f.backupDir), "evil.json")
	_ = os.WriteFile(outside, []byte(`{"vars":[{"name":"X","value":"1","type":1}]}`), 0o600)
	f.writes = nil
	for _, id := range []string{"", "x", "../evil", `..\evil`, "evil", res.BackupID + ".json", filepath.Join(f.backupDir, res.BackupID), "20990101T000000Z", "20261116T100100Z/../x"} {
		if _, err := m.Restore(id); err == nil {
			t.Errorf("%q must be refused", id)
		}
	}
	if len(f.writes) != 0 {
		t.Errorf("nothing written: %v", f.writes)
	}
}

func TestBackupsOnlyListsOurFiles(t *testing.T) {
	f := newFake(t)
	m := mgr(f)
	if l, err := m.Backups(); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("%v %v", l, err)
	}
	f.backupDir = filepath.Join(f.backupDir, "not-created-yet")
	if l, err := m.Backups(); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("a missing folder is no backups: %v %v", l, err)
	}
	_ = os.MkdirAll(f.backupDir, 0o700)
	_ = os.WriteFile(filepath.Join(f.backupDir, "notes.json"), []byte(`{}`), 0o600)
	_ = os.WriteFile(filepath.Join(f.backupDir, "20261116T100000Z.json"), []byte(`not json`), 0o600)
	_ = os.WriteFile(filepath.Join(f.backupDir, "20261116T100100Z.json.tmp"), []byte(`{}`), 0o600)
	_ = os.MkdirAll(filepath.Join(f.backupDir, "20261116T100200Z.json"), 0o700)
	if l, _ := m.Backups(); len(l) != 0 {
		t.Errorf("damaged, foreign and temporary files are not offered: %+v", l)
	}
}

func TestOnlyTenNewestBackupsAreKept(t *testing.T) {
	f := newFake(t).dir(`C:\a`)
	f.setUser("Path", `C:\a`)
	m := mgr(f)
	var first string
	for i := 0; i < 13; i++ {
		f.user["Path"] = Raw{Value: `C:\a;C:\gone`, Type: TypeString}
		res, err := m.Fix([]string{`path-missing|user|C:\gone`})
		if err != nil || res.Applied != 1 {
			t.Fatalf("%d: %+v %v", i, res, err)
		}
		if i == 0 {
			first = res.BackupID
		}
	}
	l, err := m.Backups()
	if err != nil || len(l) != 10 {
		t.Fatalf("%d backups, %v", len(l), err)
	}
	for _, b := range l {
		if b.ID == first {
			t.Error("the oldest backup is pruned")
		}
	}
	if !l[0].Time.After(l[9].Time) {
		t.Errorf("newest first: %v ... %v", l[0].Time, l[9].Time)
	}
	entries, _ := os.ReadDir(f.backupDir)
	if len(entries) != 10 {
		t.Errorf("files on disk: %d", len(entries))
	}
}

func TestTwoFixesInTheSameSecondGetDifferentBackups(t *testing.T) {
	f := newFake(t).dir(`C:\a`)
	m := &Manager{Sys: f, Now: func() time.Time { return epoch }}
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		f.user["Path"] = Raw{Value: `C:\a;C:\gone`, Type: TypeString}
		res, err := m.Fix([]string{`path-missing|user|C:\gone`})
		if err != nil || ids[res.BackupID] {
			t.Fatalf("%+v %v", res, err)
		}
		ids[res.BackupID] = true
	}
	if l, _ := m.Backups(); len(l) != 3 {
		t.Errorf("%+v", l)
	}
}

func TestFixRefusesIDsThatAreNotFixable(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	res, err := m.Fix([]string{
		`path-missing|machine|C:\machine-gone`, // machine scope
		`path-not-dir|user|C:\afile`,           // reported only
		`path-relative|user|rel`,
		`path-missing|user|C:\never-listed`, // not in the scan
		``,
		`../../etc`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 0 || len(res.Failed) != 6 || res.BackupID != "" {
		t.Errorf("%+v", res)
	}
	if f.writeCount() != 0 || f.broadcasts != 0 {
		t.Errorf("nothing written: %v", f.writes)
	}
	if l, _ := m.Backups(); len(l) != 0 {
		t.Errorf("no backup when nothing changes: %+v", l)
	}
}

func TestFixIgnoresRepeatedIDsAndStaleOnes(t *testing.T) {
	f := messyFake(t)
	m := mgr(f)
	// The PATH changes between the scan and the fix: the first segment is no longer where it was.
	res, err := m.Fix([]string{`path-missing|user|C:\gone`, `path-missing|user|C:\gone`})
	if err != nil || res.Applied != 1 || len(res.Failed) != 0 {
		t.Fatalf("a repeated id counts once: %+v %v", res, err)
	}
	res, err = m.Fix([]string{`path-missing|user|C:\gone`})
	if err != nil || res.Applied != 0 || !reflect.DeepEqual(res.Failed, []string{`path-missing|user|C:\gone`}) {
		t.Fatalf("already fixed: %+v %v", res, err)
	}
}

func TestFixRescansSoAShiftedPathNeverRemovesTheWrongSegment(t *testing.T) {
	f := newFake(t).dir(`C:\a`, `C:\b`)
	f.setUser("Path", `C:\a;C:\gone;C:\b`)
	m := mgr(f)
	// Between the person's scan and their click, a program rewrote PATH.
	f.user["Path"] = Raw{Value: `C:\gone;C:\a;C:\b`, Type: TypeString}
	res, err := m.Fix([]string{`path-missing|user|C:\gone`})
	if err != nil || res.Applied != 1 {
		t.Fatal(res, err)
	}
	if got := f.user["Path"].Value; got != `C:\a;C:\b` {
		t.Errorf("%q", got)
	}
}

func TestFixOneVariableFailingDoesNotStopTheOthers(t *testing.T) {
	f := messyFake(t)
	f.failSet["Path"] = errBoom
	m := mgr(f)
	res, err := m.Fix([]string{`path-missing|user|C:\gone`, `home-missing|user|JAVA_HOME`})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 1 || !reflect.DeepEqual(res.Failed, []string{`path-missing|user|C:\gone`}) || res.BackupID == "" {
		t.Fatalf("%+v", res)
	}
	if f.user["Path"].Value != messyPath {
		t.Error("PATH is as it was")
	}
	l, _ := m.Backups()
	if len(l) != 1 || !reflect.DeepEqual(l[0].Changed, []string{"JAVA_HOME"}) {
		t.Errorf("the backup lists only what was written: %+v", l)
	}
}

func TestFixEveryWriteFailingLeavesNoBackupAndNoBroadcast(t *testing.T) {
	f := messyFake(t)
	f.failSet["Path"] = errBoom
	m := mgr(f)
	res, err := m.Fix([]string{`path-missing|user|C:\gone`})
	if err != nil || res.Applied != 0 || res.BackupID != "" || len(res.Failed) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if l, _ := m.Backups(); len(l) != 0 || f.broadcasts != 0 {
		t.Errorf("%+v %d", l, f.broadcasts)
	}
}

func TestFixWritesNothingWhenTheBackupCannotBeSaved(t *testing.T) {
	f := messyFake(t)
	// A file where the folder should be.
	blocker := filepath.Join(t.TempDir(), "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	f.backupDir = filepath.Join(blocker, "env-backups")
	m := mgr(f)
	res, err := m.Fix([]string{`path-missing|user|C:\gone`})
	if err == nil {
		t.Fatal("must fail")
	}
	if res.Applied != 0 || res.BackupID != "" || f.writeCount() != 0 {
		t.Errorf("%+v %v", res, f.writes)
	}
	if res.Failed == nil {
		t.Error("failed is never nil")
	}
}

func TestFixWithNothingChosen(t *testing.T) {
	f := messyFake(t)
	res, err := mgr(f).Fix(nil)
	if err != nil || res.Applied != 0 || res.Failed == nil || res.BackupID != "" || f.writeCount() != 0 {
		t.Errorf("%+v %v", res, err)
	}
}

func TestFixHandlesLongPathAndManyRemovals(t *testing.T) {
	f := newFake(t)
	var parts []string
	var want []string
	var gone []string
	for i := 0; i < 120; i++ {
		d := `C:\dir\` + strings.Repeat("n", 15) + strconv.Itoa(i)
		if i%2 == 0 {
			parts = append(parts, d)
			want = append(want, d)
			f.dir(d)
		} else {
			d += `-gone`
			parts = append(parts, d)
			gone = append(gone, `path-missing|user|`+d)
		}
	}
	// 120 entries over 2000 chars, many only differing by case through repetition.
	f.user["Path"] = Raw{Value: strings.Join(parts, ";"), Type: TypeExpand}
	if len(f.user["Path"].Value) < 2000 {
		t.Fatal("test value too short")
	}
	m := mgr(f)
	res, err := m.Fix(gone)
	if err != nil || res.Applied != len(gone) {
		t.Fatalf("%+v %v", res, err)
	}
	if got := f.user["Path"].Value; got != strings.Join(want, ";") {
		t.Error("the survivors are exactly the others, in order")
	}
}
