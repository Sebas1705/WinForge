package history_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
)

var t0 = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func entry(id, app string, a history.Action, from, to string, r history.Result, at time.Time) history.Entry {
	return history.Entry{ID: id, App: app, Name: strings.ToUpper(app), Action: a, From: from, To: to, Result: r, Source: history.FromUpdate, At: at}
}

func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Apps: map[string]*catalog.App{
			"git":  {ID: "git", Name: "Git", Winget: "Git.Git"},
			"7zip": {ID: "7zip", Name: "7-Zip", Winget: "7zip.7zip", Admin: true},
		},
		Recipes: map[string]*catalog.Recipe{"tweak": {ID: "tweak", Description: "tweak", PowerShell: "x"}},
	}
}

// ---- store ----

func TestFileStoreRoundTripAndAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	s := &history.FileStore{Path: filepath.Join(dir, "sub", "history.json")}
	if all, err := s.All(); err != nil || len(all) != 0 {
		t.Fatalf("missing file must be empty: %v %v", all, err)
	}
	if err := s.Add(entry("a", "git", history.Update, "1", "2", history.OK, t0)); err != nil {
		t.Fatal(err)
	}
	all, err := s.All()
	if err != nil || len(all) != 1 || all[0].ID != "a" || !all[0].At.Equal(t0) {
		t.Fatalf("round trip: %+v %v", all, err)
	}
	// The write goes through a temporary file that is renamed, so none is left behind.
	files, _ := os.ReadDir(filepath.Dir(s.Path))
	if len(files) != 1 || files[0].Name() != "history.json" {
		t.Fatalf("leftover files: %v", files)
	}
	if err := s.SetTo(map[string]string{"a": "2.1", "zzz": "9"}); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.All(); all[0].To != "2.1" || len(all) != 1 {
		t.Fatalf("SetTo: %+v", all)
	}
}

func TestFileStoreCapKeepsTheNewest(t *testing.T) {
	s := &history.FileStore{Path: filepath.Join(t.TempDir(), "h.json"), Max: 3}
	for i := 0; i < 5; i++ {
		if err := s.Add(entry(fmt.Sprint("e", i), "git", history.Install, "", "1", history.OK, t0.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.All()
	if len(all) != 3 || all[0].ID != "e2" || all[2].ID != "e4" {
		t.Fatalf("cap: %+v", all)
	}
	if history.MaxEntries != 500 {
		t.Fatal("default cap is 500")
	}
}

func TestFileStoreDamagedFileIsQuarantinedAndNeverBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &history.FileStore{Path: path, Now: func() time.Time { return t0 }}
	all, err := s.All()
	if err != nil || len(all) != 0 {
		t.Fatalf("a damaged file reads as empty, not as an error: %v %v", all, err)
	}
	if q := s.Quarantined(); q == "" || !strings.Contains(q, "damaged") {
		t.Fatalf("quarantined: %q", q)
	} else if b, err := os.ReadFile(q); err != nil || string(b) != "{not json" {
		t.Fatalf("the damaged file must be kept as it was: %q %v", b, err)
	}
	// Recording goes on into a fresh file.
	if err := s.Add(entry("a", "git", history.Install, "", "1", history.OK, t0)); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.All(); len(all) != 1 {
		t.Fatalf("fresh file: %+v", all)
	}
}

func TestFileStoreDropsInvalidEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.json")
	body := `{"entries":[{"id":"ok","app":"git","action":"update","result":"ok","at":"2026-11-10T09:00:00Z"},
	{"id":"","app":"git","action":"update","result":"ok","at":"2026-11-10T09:00:00Z"},
	{"id":"x","app":"git","action":"format-disk","result":"ok","at":"2026-11-10T09:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := (&history.FileStore{Path: path}).All()
	if err != nil || len(all) != 1 || all[0].ID != "ok" {
		t.Fatalf("invalid entries must be dropped: %+v %v", all, err)
	}
}

func TestMemStoreBehavesLikeTheFileStore(t *testing.T) {
	m := &history.MemStore{Max: 2}
	for i := 0; i < 3; i++ {
		_ = m.Add(entry(fmt.Sprint("e", i), "git", history.Install, "", "", history.OK, t0))
	}
	_ = m.SetTo(map[string]string{"e2": "7"})
	all, _ := m.All()
	if len(all) != 2 || all[1].To != "7" {
		t.Fatalf("%+v", all)
	}
	m.Fail = errors.New("disk full")
	if m.Add(entry("x", "git", history.Install, "", "", history.OK, t0)) == nil {
		t.Fatal("Fail must fail")
	}
}

// ---- query ----

func TestQueryNewestFirstFilteredAndLimited(t *testing.T) {
	all := []history.Entry{
		entry("1", "git", history.Install, "", "1", history.OK, t0),
		entry("2", "7zip", history.Update, "23", "24", history.Failed, t0.Add(time.Hour)),
		entry("3", "git", history.Update, "1", "2", history.OK, t0.Add(2*time.Hour)),
	}
	cur := func(string) string { return "" }
	p := history.Query(all, history.Filter{}, cur)
	if p.Total != 3 || p.Entries[0].ID != "3" || p.Entries[2].ID != "1" {
		t.Fatalf("order: %+v", p.Entries)
	}
	if len(p.Apps) != 2 || p.Apps[0].ID != "7zip" || p.Apps[1].ID != "git" {
		t.Fatalf("apps sorted by name: %+v", p.Apps)
	}
	if p := history.Query(all, history.Filter{App: "git"}, cur); p.Total != 2 {
		t.Fatalf("app filter: %+v", p)
	}
	if p := history.Query(all, history.Filter{Result: "failed"}, cur); p.Total != 1 || p.Entries[0].ID != "2" {
		t.Fatalf("result filter: %+v", p)
	}
	p = history.Query(all, history.Filter{Limit: 1}, cur)
	if len(p.Entries) != 1 || p.Total != 3 || len(p.Apps) != 2 {
		t.Fatalf("limit keeps the total and the app list: %+v", p)
	}
	if p := history.Query(nil, history.Filter{}, cur); p.Entries == nil || p.Apps == nil || p.Total != 0 {
		t.Fatalf("empty must be empty lists, not null: %+v", p)
	}
}

func TestRollbackableOnlyTheLatestSuccessfulUpdateStillInstalled(t *testing.T) {
	all := []history.Entry{
		entry("u1", "git", history.Update, "1", "2", history.OK, t0),
		entry("u2", "git", history.Update, "2", "3", history.OK, t0.Add(time.Hour)),
		entry("f", "git", history.Update, "3", "", history.Failed, t0.Add(2*time.Hour)),
		entry("i", "7zip", history.Install, "", "23", history.OK, t0),
		entry("n", "vlc", history.Update, "", "4", history.OK, t0), // previous version unknown
		entry("p", "obs", history.Update, "5", "6", history.OK, t0),
		entry("r", "obs", history.Rollback, "6", "5", history.OK, t0.Add(time.Hour)),
	}
	versions := map[string]string{"git": "3", "7zip": "23", "vlc": "4", "obs": "5"}
	ok := history.Rollbackable(all, func(a string) string { return versions[a] })
	if len(ok) != 1 || !ok["u2"] {
		t.Fatalf("only u2 can be undone (a failed step does not hide it): %v", ok)
	}
	versions["git"] = "4" // updated outside WinForge since
	if ok := history.Rollbackable(all, func(a string) string { return versions[a] }); len(ok) != 0 {
		t.Fatalf("the app is no longer at the version that update installed: %v", ok)
	}
}

// ---- rollback plan ----

func TestRollbackPlanPinsTheVersionAndNeedsNoOtherPath(t *testing.T) {
	cat := testCatalog()
	e := entry("u", "7zip", history.Update, "23.01", "24.08", history.OK, t0)
	p, err := history.RollbackPlan(cat, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 || p.Steps[0] != (install.Step{Kind: install.StepRollback, ID: "7zip", Name: "7-Zip", Version: "23.01", Admin: true}) || !p.NeedsAdmin {
		t.Fatalf("plan: %+v", p)
	}
	args := strings.Join(install.WingetRollbackArgs(cat.Apps["7zip"], "23.01"), " ")
	for _, want := range []string{"install --id 7zip.7zip", "--exact", "--version 23.01", "--force"} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
}

func TestRollbackPlanRefusesWhatIsNotAnUndoableUpdate(t *testing.T) {
	cat := testCatalog()
	for name, e := range map[string]history.Entry{
		"install":        entry("a", "git", history.Install, "", "2", history.OK, t0),
		"failed":         entry("b", "git", history.Update, "1", "", history.Failed, t0),
		"no previous":    entry("c", "git", history.Update, "", "2", history.OK, t0),
		"not in catalog": entry("d", "ghost", history.Update, "1", "2", history.OK, t0),
		"option-looking": entry("e", "git", history.Update, "--source evil", "2", history.OK, t0),
		"rollback":       entry("f", "git", history.Rollback, "2", "1", history.OK, t0),
	} {
		if _, err := history.RollbackPlan(cat, e); !errors.Is(err, history.ErrNotRollbackable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestFindAcceptsOnlyIdsFromTheHistory(t *testing.T) {
	all := []history.Entry{entry("a", "git", history.Update, "1", "2", history.OK, t0)}
	if _, err := history.Find(all, "a"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "git", "b", "../a"} {
		if _, err := history.Find(all, id); !errors.Is(err, history.ErrUnknownEntry) {
			t.Errorf("%q: %v", id, err)
		}
	}
}

// ---- recording from real runs, with a fake executor ----

type fakeExec struct {
	calls []string
	exit  map[string]int
	lines map[string]string
}

func (f *fakeExec) Run(_ context.Context, name string, args []string, onLine func(string)) (int, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for k, v := range f.lines {
		if strings.Contains(line, k) {
			onLine(v)
		}
	}
	for k, code := range f.exit {
		if strings.Contains(line, k) {
			return code, nil
		}
	}
	return 0, nil
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { c.t = c.t.Add(time.Second); return c.t }

// record runs a plan through install.Run, feeding every event to a recording, like the app does.
func record(t *testing.T, plan install.Plan, title string, before map[string]string, ex install.Executor) (*history.MemStore, *history.Recording) {
	t.Helper()
	store := &history.MemStore{}
	c := &clock{t0}
	rec := (&history.Recorder{Store: store, Now: c.now}).Begin(plan, title, before)
	install.Run(context.Background(), testCatalog(), plan, ex, rec.Observe)
	return store, rec
}

func TestRecordsAnUpdateRunWithVersionsAndSource(t *testing.T) {
	plan := install.Plan{Steps: []install.Step{
		{Kind: install.StepUpgrade, ID: "git", Name: "Git", Version: "2.0"},
		{Kind: install.StepUpgrade, ID: "7zip", Name: "7-Zip", Version: "24"},
	}}
	ex := &fakeExec{exit: map[string]int{"7zip.7zip": 3010}}
	store, rec := record(t, plan, "updates", map[string]string{"git": "1.0", "7zip": "23"}, ex)
	all, _ := store.All()
	if len(all) != 2 {
		t.Fatalf("entries: %+v", all)
	}
	git, z := all[0], all[1]
	if git.Action != history.Update || git.From != "1.0" || git.To != "2.0" || git.Result != history.OK || git.Source != history.FromUpdate || git.Name != "Git" {
		t.Errorf("git: %+v", git)
	}
	if z.Result != history.Restart {
		t.Errorf("a restart-needed step is recorded as such: %+v", z)
	}
	if git.ID == "" || git.ID == z.ID || !git.At.Before(z.At) {
		t.Errorf("ids and times: %+v %+v", git, z)
	}
	// One scan after the run fills in what the apps really ended on.
	if !rec.NeedsDetect() {
		t.Fatal("successful steps need the versions read")
	}
	rec.Finish(map[string]string{"git": "2.0.1", "7zip": ""})
	all, _ = store.All()
	if all[0].To != "2.0.1" || all[1].To != "24" {
		t.Errorf("after Finish: %+v", all)
	}
}

func TestRecordsAProfileInstallAndASingleApp(t *testing.T) {
	two := install.Plan{Steps: []install.Step{
		{Kind: install.StepApp, ID: "git", Name: "Git", Version: "2.0"},
		{Kind: install.StepApp, ID: "7zip", Name: "7-Zip"},
		{Kind: install.StepRecipe, ID: "tweak", Name: "tweak"},
	}}
	store, _ := record(t, two, "My profile", map[string]string{"git": "stale"}, &fakeExec{})
	all, _ := store.All()
	if len(all) != 2 {
		t.Fatalf("a recipe is not app history: %+v", all)
	}
	if all[0].Source != history.FromProfile || all[0].Action != history.Install || all[0].From != "" || all[0].To != "2.0" {
		t.Errorf("profile install: %+v", all[0])
	}
	one := install.Plan{Steps: []install.Step{{Kind: install.StepApp, ID: "git", Name: "Git"}}}
	store, _ = record(t, one, "Git", nil, &fakeExec{})
	if all, _ := store.All(); all[0].Source != history.FromSingle {
		t.Errorf("one app is a single install: %+v", all[0])
	}
}

func TestRecordsAFailureWithItsReasonAndSkipsWhatDidNotRun(t *testing.T) {
	plan := install.Plan{Steps: []install.Step{
		{Kind: install.StepUpgrade, ID: "git", Name: "Git", Version: "2.0"},
		{Kind: install.StepUpgrade, ID: "7zip", Name: "7-Zip", Version: "24"},
	}}
	ex := &fakeExec{exit: map[string]int{"Git.Git": int(int32(-1978335224))}} // 0x8A150008: download failed
	ctx, cancel := context.WithCancel(context.Background())
	store := &history.MemStore{}
	rec := (&history.Recorder{Store: store, Now: (&clock{t0}).now}).Begin(plan, "updates", map[string]string{"git": "1.0"})
	install.Run(ctx, testCatalog(), plan, ex, func(e install.Event) {
		rec.Observe(e)
		if e.Status == install.StatusFailed {
			cancel() // the second step is skipped
		}
	})
	all, _ := store.All()
	if len(all) != 1 {
		t.Fatalf("a skipped step did nothing and is not recorded: %+v", all)
	}
	if all[0].Result != history.Failed || all[0].Reason != install.ReasonNetwork || all[0].To != "" || all[0].From != "1.0" {
		t.Errorf("failure: %+v", all[0])
	}
	if rec.NeedsDetect() {
		t.Error("nothing succeeded, nothing to read")
	}
}

func TestRecordsARollbackAndItsFailureReasons(t *testing.T) {
	plan, err := history.RollbackPlan(testCatalog(), entry("u", "git", history.Update, "1.0", "2.0", history.OK, t0))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		ex     *fakeExec
		result history.Result
		reason string
	}{
		{"works", &fakeExec{}, history.OK, ""},
		{"restart", &fakeExec{exit: map[string]int{"--version": 3010}}, history.Restart, ""},
		{"winget says it is already there", &fakeExec{exit: map[string]int{"--version": int(int32(-1978335189))}}, history.Failed, install.ReasonDowngrade},
		{"already installed code", &fakeExec{exit: map[string]int{"--version": int(int32(-1978335135))}}, history.Failed, install.ReasonDowngrade},
		{"no such version", &fakeExec{exit: map[string]int{"--version": 1}, lines: map[string]string{"--version": "No version found matching: 1.0"}}, history.Failed, install.ReasonVersion},
		{"installer refuses", &fakeExec{exit: map[string]int{"--version": 1603}, lines: map[string]string{"--version": "Installer failed with exit code: 1638"}}, history.Failed, install.ReasonDowngrade},
		{"download", &fakeExec{exit: map[string]int{"--version": int(int32(-1978335224))}}, history.Failed, install.ReasonNetwork},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, _ := record(t, plan, "rollback", map[string]string{"git": "2.0"}, c.ex)
			all, _ := store.All()
			if len(all) != 1 {
				t.Fatalf("entries: %+v", all)
			}
			e := all[0]
			if e.Action != history.Rollback || e.Source != history.FromSingle || e.From != "2.0" || e.Result != c.result || e.Reason != c.reason {
				t.Fatalf("%+v", e)
			}
			if c.result != history.Failed && e.To != "1.0" {
				t.Errorf("to: %+v", e)
			}
			if call := c.ex.calls[0]; !strings.Contains(call, "--version 1.0") || !strings.Contains(call, "--force") || !strings.Contains(call, "install") {
				t.Errorf("the rollback must be a pinned, forced install: %s", call)
			}
		})
	}
}

func TestRecordingNeverBlocksARun(t *testing.T) {
	var heard error
	store := &history.MemStore{Fail: errors.New("disk full")}
	plan := install.Plan{Steps: []install.Step{{Kind: install.StepApp, ID: "git", Name: "Git"}}}
	rec := (&history.Recorder{Store: store, OnError: func(e error) { heard = e }}).Begin(plan, "Git", nil)
	if failed := install.Run(context.Background(), testCatalog(), plan, &fakeExec{}, rec.Observe); len(failed) != 0 {
		t.Fatalf("run failed: %v", failed)
	}
	if heard == nil {
		t.Fatal("the store error must be reported, not swallowed")
	}
	if rec.NeedsDetect() {
		t.Fatal("nothing was stored")
	}
	rec.Finish(map[string]string{"git": "1"}) // must not panic nor write
}

func TestNilRecorderAndRecordingRecordNothing(t *testing.T) {
	var r *history.Recorder
	rec := r.Begin(install.Plan{}, "x", nil)
	rec.Observe(install.Event{Step: install.Step{Kind: install.StepApp, ID: "git"}, Status: install.StatusOK})
	rec.Finish(nil)
	if rec.NeedsDetect() {
		t.Fatal("nil records nothing")
	}
	if (&history.Recorder{}).Begin(install.Plan{}, "x", nil) != nil {
		t.Fatal("no store, no recording")
	}
}

func TestRecordingAgainstTheFileStore(t *testing.T) {
	s := &history.FileStore{Path: filepath.Join(t.TempDir(), "h.json")}
	plan := install.Plan{Steps: []install.Step{{Kind: install.StepUpgrade, ID: "git", Name: "Git", Version: "2"}}}
	rec := (&history.Recorder{Store: s}).Begin(plan, "updates", map[string]string{"git": "1"})
	install.Run(context.Background(), testCatalog(), plan, &fakeExec{}, rec.Observe)
	rec.Finish(map[string]string{"git": "2.0.1"})
	all, _ := s.All()
	if len(all) != 1 || all[0].From != "1" || all[0].To != "2.0.1" {
		t.Fatalf("%+v", all)
	}
}
