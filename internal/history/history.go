// Package history remembers what WinForge did to catalog apps (installed, updated, rolled back),
// so the person can see it later and put an app back on the version it had before an update.
//
// Everything here is pure logic over a Store; the app records into it from the one place that
// sees every finished step (see Recorder), and never installs anything itself.
package history

import (
	"sort"
	"strings"
	"time"
)

// Action is what was done to the app.
type Action string

const (
	Install  Action = "install"
	Update   Action = "update"
	Rollback Action = "rollback"
)

// Result is how the step ended.
type Result string

const (
	OK      Result = "ok"
	Failed  Result = "failed"
	Restart Result = "restart" // done, but Windows has to restart to finish
)

// Source is which part of the app asked for it.
type Source string

const (
	FromProfile Source = "profile" // applying a profile or a multi-app selection
	FromUpdate  Source = "update"  // the Updates page
	FromSingle  Source = "single"  // one app on its own, including a rollback
)

// MaxEntries is how many entries are kept; the oldest go first.
const MaxEntries = 500

// Entry is one step that finished.
type Entry struct {
	ID     string    `json:"id"`
	App    string    `json:"app"`  // catalog id
	Name   string    `json:"name"` // catalog name when it happened
	Action Action    `json:"action"`
	From   string    `json:"from"` // version before; "" when none or unknown
	To     string    `json:"to"`   // version after; "" when unknown
	At     time.Time `json:"at"`
	Result Result    `json:"result"`
	Source Source    `json:"source"`
	// Reason is why a failed step failed, as the code install.Explain gives; "" when unknown.
	Reason string `json:"reason,omitempty"`
}

// Valid says whether a loaded entry can be shown. A damaged one is dropped, never repaired.
func (e Entry) Valid() bool {
	switch e.Action {
	case Install, Update, Rollback:
	default:
		return false
	}
	switch e.Result {
	case OK, Failed, Restart:
	default:
		return false
	}
	return e.ID != "" && e.App != "" && !e.At.IsZero()
}

// Store keeps the entries. Entries are listed oldest first.
type Store interface {
	// Add appends entries and drops the oldest beyond MaxEntries.
	Add(entries ...Entry) error
	// All returns every entry, oldest first.
	All() ([]Entry, error)
	// SetTo fills in the version after, by entry id. Unknown ids are ignored.
	SetTo(to map[string]string) error
}

// Filter narrows a listing. Empty fields match everything.
type Filter struct {
	App    string `json:"app"`
	Result string `json:"result"`
	// Limit is how many entries to return; 0 means all.
	Limit int `json:"limit"`
}

// View is an entry as the UI lists it.
type View struct {
	Entry
	// CanRollback is true on an update whose previous version is known while the app is still
	// at the version that update installed.
	CanRollback bool `json:"canRollback"`
}

// AppRef names an app that appears in the history, for the filter.
type AppRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Page is a listing: the newest entries first, how many matched in all, and every app that has
// any history (not only those on this page).
type Page struct {
	Entries []View   `json:"entries"`
	Total   int      `json:"total"`
	Apps    []AppRef `json:"apps"`
}

// Rollbackable says which entries could be undone right now: the newest update of each app, only
// when it succeeded, its previous version is known and the app is still at the version it
// installed. current gives an app's installed version, "" when unknown.
func Rollbackable(entries []Entry, current func(app string) string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]bool{}
	for i := len(entries) - 1; i >= 0; i-- { // newest first
		e := entries[i]
		if e.Action == Install || e.Result == Failed || seen[e.App] {
			continue
		}
		seen[e.App] = true // only the latest change to an app can be undone
		if e.Action == Update && e.From != "" && e.To != "" && current(e.App) == e.To {
			out[e.ID] = true
		}
	}
	return out
}

// Query lists the history: newest first, filtered, cut to the limit.
func Query(entries []Entry, f Filter, current func(app string) string) Page {
	ok := Rollbackable(entries, current)
	sorted := append([]Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At.After(sorted[j].At) })

	names := map[string]string{}
	for _, e := range sorted { // newest name wins
		if _, have := names[e.App]; !have {
			names[e.App] = e.Name
		}
	}
	page := Page{Entries: []View{}, Apps: []AppRef{}}
	for id, name := range names {
		page.Apps = append(page.Apps, AppRef{ID: id, Name: name})
	}
	sort.Slice(page.Apps, func(i, j int) bool {
		a, b := strings.ToLower(page.Apps[i].Name), strings.ToLower(page.Apps[j].Name)
		if a != b {
			return a < b
		}
		return page.Apps[i].ID < page.Apps[j].ID
	})
	for _, e := range sorted {
		if f.App != "" && e.App != f.App {
			continue
		}
		if f.Result != "" && string(e.Result) != f.Result {
			continue
		}
		page.Total++
		if f.Limit <= 0 || len(page.Entries) < f.Limit {
			page.Entries = append(page.Entries, View{Entry: e, CanRollback: ok[e.ID]})
		}
	}
	return page
}
