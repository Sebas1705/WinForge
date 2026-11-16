package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// historySession holds what a rollback in progress has to do once it succeeds.
type historySession struct {
	mu sync.Mutex
	// skip maps an app to the newer version that should be skipped in the ignore rules when its
	// rollback succeeds. It is only set when the person asked for it.
	skip map[string]string
}

func (a *App) historyRecorder() *history.Recorder {
	if a.history == nil {
		return nil
	}
	return &history.Recorder{Store: a.history, Now: a.clock, OnError: func(err error) { log.Printf("winforge: history: %v", err) }}
}

// historyBegin starts writing a run down. It is called by start, the one place every run goes
// through, and returns nil (which records nothing) when there is no history store.
func (a *App) historyBegin(plan install.Plan, title string) *history.Recording {
	rec := a.historyRecorder()
	if rec == nil {
		return nil
	}
	before := map[string]string{}
	a.mu.Lock()
	for id, i := range a.installed {
		before[id] = i.Version
	}
	// An update step starts from the version the last update check saw, the same one the page showed.
	for _, s := range plan.Steps {
		if s.Kind != install.StepUpgrade || a.cat.Apps[s.ID] == nil {
			continue
		}
		for _, u := range a.upgrades {
			if u.ID == a.cat.Apps[s.ID].Winget && u.Current != "" {
				before[s.ID] = u.Current
			}
		}
	}
	a.mu.Unlock()
	r := rec.Begin(plan, title, before)
	if r != nil {
		r.OnEntry = a.historyRecorded
	}
	return r
}

// historyRecorded runs for every entry that was stored. After a rollback it applies the
// "skip this version" choice, but only if the rollback worked: a failed one changes no rules.
func (a *App) historyRecorded(e history.Entry) {
	if e.Action != history.Rollback {
		return
	}
	a.historyState.mu.Lock()
	skip, ok := a.historyState.skip[e.App]
	delete(a.historyState.skip, e.App)
	a.historyState.mu.Unlock()
	if !ok || e.Result == history.Failed || a.ignored == nil {
		return
	}
	if err := a.ignored.Set(e.App, updates.Rule{Mode: updates.Version, Version: skip}); err != nil {
		log.Printf("winforge: history: could not skip version %s: %v", skip, err)
	}
}

// historyFinish runs once a run is over. If something was installed or updated it reads the PC once, with
// the detection the pages use, to fill in the versions the apps ended on, and keeps that scan as the
// current one so "go back" is offered right away.
func (a *App) historyFinish(r *history.Recording) {
	if !r.NeedsDetect() {
		return
	}
	var found map[string]system.Installed
	if a.detect != nil {
		found = a.detect()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		inv, _ := system.Snapshot(ctx) // without winget the registry and PATH still say something
		found = system.Detect(a.cat, inv)
	}
	if found == nil {
		return
	}
	versions := make(map[string]string, len(found))
	for id, i := range found {
		versions[id] = i.Version
	}
	r.Finish(versions)
	a.mu.Lock()
	a.installed = found
	a.mu.Unlock()
}

// installedVersion is the version an app is installed at, per the last scan.
func (a *App) installedVersion(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.installed[id].Version
}

// HistoryList lists what WinForge did to catalog apps, newest first. Each update that can be
// undone says so.
func (a *App) HistoryList(f history.Filter) (history.Page, error) {
	if a.history == nil {
		return history.Page{Entries: []history.View{}, Apps: []history.AppRef{}}, nil
	}
	all, err := a.history.All()
	if err != nil {
		return history.Page{}, err
	}
	return history.Query(all, f, a.installedVersion), nil
}

// historyRollback finds the entry and checks that it can be undone now, and builds the plan.
func (a *App) historyRollback(id string) (history.Entry, install.Plan, error) {
	if a.history == nil {
		return history.Entry{}, install.Plan{}, history.ErrUnknownEntry
	}
	all, err := a.history.All()
	if err != nil {
		return history.Entry{}, install.Plan{}, err
	}
	e, err := history.Find(all, id)
	if err != nil {
		return history.Entry{}, install.Plan{}, err
	}
	if !history.Rollbackable(all, a.installedVersion)[e.ID] {
		if e.Action == history.Update && e.Result != history.Failed && e.To != "" && a.installedVersion(e.App) != e.To {
			return e, install.Plan{}, history.ErrMoved
		}
		return e, install.Plan{}, history.ErrNotRollbackable
	}
	plan, err := history.RollbackPlan(a.cat, e)
	return e, plan, err
}

// HistoryRollbackPlan previews what HistoryRollback would run, for the confirmation window.
func (a *App) HistoryRollbackPlan(id string) (install.Plan, error) {
	_, plan, err := a.historyRollback(id)
	return plan, err
}

// HistoryRollback puts an app back on the version it had before an update, through the same runner
// as every install: one step that installs that exact version. It only accepts ids from the
// history. With skipNewer, once the rollback has worked the version it replaced is added to the
// ignore rules, so the app is not updated straight back.
func (a *App) HistoryRollback(id string, skipNewer bool) error {
	e, plan, err := a.historyRollback(id)
	if err != nil {
		return err
	}
	if skipNewer && a.ignored == nil {
		return errNoIgnoreStore
	}
	a.historyState.mu.Lock()
	if a.historyState.skip == nil {
		a.historyState.skip = map[string]string{}
	}
	delete(a.historyState.skip, e.App)
	if skipNewer {
		a.historyState.skip[e.App] = e.To
	}
	a.historyState.mu.Unlock()
	if err := a.start(plan, "rollback", ""); err != nil {
		a.historyState.mu.Lock()
		delete(a.historyState.skip, e.App)
		a.historyState.mu.Unlock()
		return err
	}
	return nil
}
