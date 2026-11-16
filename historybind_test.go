package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// Nothing here calls start (and so never runs winget): the plan, the allow-list and the recording
// are driven directly, and the real runner is covered in internal/history with a fake executor.

func histApp(t *testing.T) (*App, *history.MemStore) {
	t.Helper()
	a := testApp(t)
	st := &history.MemStore{}
	a.history = st
	a.ignored = &updates.Store{Path: filepath.Join(t.TempDir(), "ignored-updates.json")}
	a.now = func() time.Time { return day0 }
	a.installed = map[string]system.Installed{"git": {ID: "git", Version: "2.0"}, "vlc": {ID: "vlc", Version: "3.0"}}
	return a, st
}

func updateEntry(id, app, from, to string, r history.Result) history.Entry {
	return history.Entry{ID: id, App: app, Name: app, Action: history.Update, From: from, To: to, Result: r, Source: history.FromUpdate, At: day0}
}

func TestHistoryListMarksTheUndoableUpdate(t *testing.T) {
	a, st := histApp(t)
	_ = st.Add(updateEntry("u1", "git", "1.0", "2.0", history.OK), updateEntry("u2", "vlc", "2.0", "2.5", history.OK))
	page, err := a.HistoryList(history.Filter{})
	if err != nil || page.Total != 2 {
		t.Fatalf("%+v %v", page, err)
	}
	can := map[string]bool{}
	for _, v := range page.Entries {
		can[v.ID] = v.CanRollback
	}
	if !can["u1"] || can["u2"] {
		t.Fatalf("git is at 2.0 so u1 can be undone; vlc moved on to 3.0 so u2 cannot: %v", can)
	}
	a.history = nil
	if page, err := a.HistoryList(history.Filter{}); err != nil || page.Entries == nil || len(page.Entries) != 0 {
		t.Fatalf("no store is an empty history: %+v %v", page, err)
	}
}

func TestHistoryRollbackBuildsItsOwnPlanFromAHistoryEntry(t *testing.T) {
	a, st := histApp(t)
	_ = st.Add(updateEntry("u1", "git", "1.0", "2.0", history.OK))
	plan, err := a.HistoryRollbackPlan("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Kind != install.StepRollback || plan.Steps[0].ID != "git" || plan.Steps[0].Version != "1.0" {
		t.Fatalf("plan: %+v", plan)
	}
}

func TestHistoryRollbackAllowList(t *testing.T) {
	a, st := histApp(t)
	_ = st.Add(
		updateEntry("u1", "git", "1.0", "2.0", history.OK),
		updateEntry("moved", "vlc", "2.0", "2.5", history.OK),
		updateEntry("bad", "git", "1.0", "", history.Failed),
		history.Entry{ID: "inst", App: "git", Name: "git", Action: history.Install, To: "2.0", Result: history.OK, Source: history.FromSingle, At: day0},
	)
	for id, want := range map[string]error{
		"git":   history.ErrUnknownEntry, // an app id, not an entry id
		"":      history.ErrUnknownEntry,
		"nope":  history.ErrUnknownEntry,
		"moved": history.ErrMoved,
		"bad":   history.ErrNotRollbackable,
		"inst":  history.ErrNotRollbackable,
	} {
		if err := a.HistoryRollback(id, false); !errors.Is(err, want) {
			t.Errorf("%q: %v, want %v", id, err, want)
		}
	}
	if _, err := a.HistoryRollbackPlan("nope"); !errors.Is(err, history.ErrUnknownEntry) {
		t.Errorf("plan: %v", err)
	}
	a.history = nil
	if err := a.HistoryRollback("u1", false); !errors.Is(err, history.ErrUnknownEntry) {
		t.Errorf("no store: %v", err)
	}
}

func TestHistoryRollbackRefusesSkipWhenRulesCannotBeSaved(t *testing.T) {
	a, st := histApp(t)
	_ = st.Add(updateEntry("u1", "git", "1.0", "2.0", history.OK))
	a.ignored = nil
	wantErr(t, a.HistoryRollback("u1", true), "ignored updates cannot be saved")
	if a.historyState.skip["git"] != "" {
		t.Fatal("nothing must be left pending")
	}
}

func rollbackDone(app, from, to string, r history.Result) history.Entry {
	return history.Entry{ID: "r", App: app, Action: history.Rollback, From: from, To: to, Result: r, Source: history.FromSingle, At: day0}
}

func TestSkipVersionRuleIsAddedOnlyWhenAskedAndOnlyAfterSuccess(t *testing.T) {
	a, _ := histApp(t)
	rules := func() map[string]updates.Rule { r, _ := a.ignoreRules(); return r }

	// asked, and it worked
	a.historyState.skip = map[string]string{"git": "2.0"}
	a.historyRecorded(rollbackDone("git", "2.0", "1.0", history.OK))
	if r := rules()["git"]; r.Mode != updates.Version || r.Version != "2.0" {
		t.Fatalf("rule: %+v", r)
	}
	if !rules()["git"].Hides("2.0", day0) || rules()["git"].Hides("2.1", day0) {
		t.Fatal("only the version that was rolled back from is skipped")
	}
	// asked, but it failed: no rule, and the request is forgotten
	_ = a.ignored.Remove("git")
	a.historyState.skip = map[string]string{"git": "2.0"}
	a.historyRecorded(rollbackDone("git", "2.0", "", history.Failed))
	if len(rules()) != 0 || len(a.historyState.skip) != 0 {
		t.Fatalf("a failed rollback changes no rules: %v %v", rules(), a.historyState.skip)
	}
	// not asked: no rule
	a.historyRecorded(rollbackDone("git", "2.0", "1.0", history.OK))
	if len(rules()) != 0 {
		t.Fatalf("no rule unless asked: %v", rules())
	}
	// a restart-needed rollback still worked
	a.historyState.skip = map[string]string{"git": "2.0"}
	a.historyRecorded(rollbackDone("git", "2.0", "1.0", history.Restart))
	if len(rules()) != 1 {
		t.Fatalf("restart counts as done: %v", rules())
	}
	// other kinds of entry never touch the rules
	_ = a.ignored.Remove("git")
	a.historyState.skip = map[string]string{"git": "2.0"}
	a.historyRecorded(updateEntry("x", "git", "1.0", "2.0", history.OK))
	if len(rules()) != 0 {
		t.Fatalf("%v", rules())
	}
}

func TestHistoryBeginReadsBeforeVersionsAndFinishFillsAfter(t *testing.T) {
	a, st := histApp(t)
	a.upgrades = []system.Upgrade{{ID: "Git.Git", Current: "1.9", Available: "2.0"}}
	a.detect = func() map[string]system.Installed {
		return map[string]system.Installed{"git": {ID: "git", Version: "2.0.3"}, "vlc": {ID: "vlc", Version: "3.0"}}
	}
	plan := install.Plan{Steps: []install.Step{{Kind: install.StepUpgrade, ID: "git", Name: "Git", Version: "2.0"}}}
	rec := a.historyBegin(plan, "updates")
	rec.Observe(install.Event{Step: plan.Steps[0], Status: install.StatusStart})
	rec.Observe(install.Event{Step: plan.Steps[0], Status: install.StatusOutput, Line: "x"})
	rec.Observe(install.Event{Step: plan.Steps[0], Status: install.StatusOK})
	a.historyFinish(rec)
	all, _ := st.All()
	if len(all) != 1 {
		t.Fatalf("one finished step, one entry: %+v", all)
	}
	e := all[0]
	if e.From != "1.9" || e.To != "2.0.3" || e.Source != history.FromUpdate || e.Action != history.Update {
		t.Fatalf("entry: %+v", e)
	}
	if a.installed["git"].Version != "2.0.3" {
		t.Fatalf("the scan after the run is kept: %+v", a.installed)
	}
	// and the update that just happened can be undone right away
	if _, err := a.HistoryRollbackPlan(e.ID); err != nil {
		t.Fatalf("go back is offered after the run: %v", err)
	}
}

func TestNoHistoryStoreMeansNoRecordingAndNoScan(t *testing.T) {
	a := testApp(t)
	a.detect = func() map[string]system.Installed { t.Fatal("must not scan"); return nil }
	rec := a.historyBegin(install.Plan{}, "updates")
	if rec != nil {
		t.Fatal("no store, no recording")
	}
	rec.Observe(install.Event{})
	a.historyFinish(rec)
}
