package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/drift"
	"github.com/Sebas1705/WinForge/internal/system"
)

func inst(pairs ...string) map[string]system.Installed {
	m := map[string]system.Installed{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = system.Installed{ID: pairs[i], Version: pairs[i+1]}
	}
	return m
}

// driftApp has a built-in profile "base" with Git and VLC, a clock the test moves, and a history store.
func driftApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	a.cat.Profiles["base"].Apps = []catalog.ProfileApp{{ID: "git"}, {ID: "vlc"}}
	a.applied = &drift.Store{Path: filepath.Join(t.TempDir(), "applied.json")}
	day := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return day }
	return a
}

func TestAProfileThatWasNeverAppliedHasNothingToCompare(t *testing.T) {
	a := driftApp(t)
	a.installed = inst("git", "2")
	info, err := a.ProfileDrift("base")
	if err != nil || info.HasBaseline || info.Count != 0 || info.Removed == nil || info.Added == nil || info.Updated == nil {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestAfterAFullApplyTheNextScanBecomesTheBaseline(t *testing.T) {
	a := driftApp(t)
	a.baselineFor = "base" // set when a whole profile finishes without a failure
	a.rememberBaseline(inst("git", "2", "vlc", "3", "7zip", "23"))
	if a.baselineFor != "" {
		t.Fatal("the baseline is taken once")
	}
	a.rememberBaseline(inst("git", "99")) // a later scan must not overwrite it
	a.installed = inst("git", "2", "vlc", "3", "7zip", "23")
	info, err := a.ProfileDrift("base")
	if err != nil || !info.HasBaseline || info.Count != 0 || info.At != "2026-11-15T09:00:00Z" {
		t.Fatalf("%+v %v", info, err)
	}
}

func TestDriftNamesWhatChanged(t *testing.T) {
	a := driftApp(t)
	a.baselineFor = "base"
	a.rememberBaseline(inst("git", "2", "vlc", "3"))
	a.installed = inst("git", "2.1", "7zip", "24") // VLC removed, Git updated, 7-Zip new
	info, err := a.ProfileDrift("base")
	if err != nil || info.Count != 3 {
		t.Fatalf("%+v %v", info, err)
	}
	if len(info.Removed) != 1 || info.Removed[0] != (DriftApp{ID: "vlc", Name: "VLC"}) {
		t.Fatalf("removed: %+v", info.Removed)
	}
	if len(info.Added) != 1 || info.Added[0] != (DriftApp{ID: "7zip", Name: "7-Zip"}) {
		t.Fatalf("added: %+v", info.Added)
	}
	if len(info.Updated) != 1 || info.Updated[0] != (DriftChange{ID: "git", Name: "Git", From: "2", To: "2.1"}) {
		t.Fatalf("updated: %+v", info.Updated)
	}
}

func TestAcceptingTheStateStopsReportingTheChanges(t *testing.T) {
	a := driftApp(t)
	a.baselineFor = "base"
	a.rememberBaseline(inst("git", "2", "vlc", "3"))
	a.installed = inst("git", "2", "7zip", "24")
	if info, _ := a.ProfileDrift("base"); info.Count == 0 {
		t.Fatal("there is drift to accept")
	}
	if err := a.AcceptProfileState("base"); err != nil {
		t.Fatal(err)
	}
	if info, _ := a.ProfileDrift("base"); info.Count != 0 || !info.HasBaseline {
		t.Fatalf("%+v", info)
	}
	// accepting changed nothing on the PC
	if len(a.installed) != 2 {
		t.Fatal("the scan result must not be touched")
	}
}

func TestAcceptingNeedsAKnownProfileAndAScan(t *testing.T) {
	a := driftApp(t)
	a.installed = inst("git", "2")
	wantErr(t, a.AcceptProfileState("ghost"), "no profile")
	a.installed = nil
	wantErr(t, a.AcceptProfileState("base"), "not been scanned")
	_, err := a.ProfileDrift("base")
	if err != nil {
		t.Fatalf("no baseline yet: nothing to compare, so a scan is not needed: %v", err)
	}
	a.baselineFor = "base"
	a.rememberBaseline(inst("git", "2"))
	_, err = a.ProfileDrift("base")
	wantErr(t, err, "not been scanned")
}

func TestOnlyAWholeProfileCountsAsApplied(t *testing.T) {
	a := driftApp(t)
	whole := catalog.Profile{ID: "base", Name: "Base", Apps: []catalog.ProfileApp{{ID: "vlc"}, {ID: "git"}}} // same apps, any order
	if got := a.wholeProfileID(whole); got != "base" {
		t.Fatalf("the whole profile: %q", got)
	}
	subset := catalog.Profile{ID: "base", Name: "Base", Apps: []catalog.ProfileApp{{ID: "git"}}}
	if got := a.wholeProfileID(subset); got != "" {
		t.Fatalf("a subset is not the profile: %q", got)
	}
	bigger := catalog.Profile{ID: "base", Name: "Base", Apps: []catalog.ProfileApp{{ID: "git"}, {ID: "vlc"}, {ID: "7zip"}}}
	if got := a.wholeProfileID(bigger); got != "" {
		t.Fatalf("an edited copy is not the profile: %q", got)
	}
	// installing one app from the catalog builds a selection named after the app; if that id happens to match a
	// profile, it must not pass for it
	selection := catalog.Profile{ID: "base", Name: "Base", Apps: []catalog.ProfileApp{{ID: "git"}}}
	if a.wholeProfileID(selection) != "" {
		t.Fatal("a selection that shares a profile's id must not rewrite its history")
	}
	unknown := catalog.Profile{ID: "nope", Name: "Nope", Apps: []catalog.ProfileApp{{ID: "git"}}}
	if a.wholeProfileID(unknown) != "" {
		t.Fatal("a profile that does not exist has no history")
	}
}

func TestThePersonsOwnProfilesHaveAHistoryToo(t *testing.T) {
	a := driftApp(t)
	if err := a.SaveProfile(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git"}}}); err != nil {
		t.Fatal(err)
	}
	if got := a.wholeProfileID(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git"}}}); got != "mine" {
		t.Fatalf("%q", got)
	}
	a.baselineFor = "mine"
	a.rememberBaseline(inst("git", "2"))
	a.installed = inst()
	info, err := a.ProfileDrift("mine")
	if err != nil || len(info.Removed) != 1 || info.Removed[0].ID != "git" {
		t.Fatalf("%+v %v", info, err)
	}
	// deleting the profile forgets its history
	if err := a.DeleteProfile("mine"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := a.applied.Get("mine"); ok {
		t.Fatal("a deleted profile keeps no history")
	}
}

func TestCountsAreAttachedToTheProfilesThatHaveAHistory(t *testing.T) {
	a := driftApp(t)
	a.baselineFor = "base"
	a.rememberBaseline(inst("git", "2", "vlc", "3"))
	counts := a.driftCounts(map[string][]string{"base": {"git", "vlc"}, "other": {"git"}}, inst("git", "2"))
	if len(counts) != 1 || counts["base"] != 1 {
		t.Fatalf("%v", counts)
	}
	st := State{Profiles: []ProfileInfo{{Profile: catalog.Profile{ID: "base"}, Resolved: []string{"git", "vlc"}}, {Profile: catalog.Profile{ID: "other"}, Resolved: []string{"git"}}}}
	a.attachDrift(&st, inst("git", "2"))
	if st.Profiles[0].Changes != 1 || st.Profiles[1].Changes != 0 {
		t.Fatalf("%+v", st.Profiles)
	}
}

func TestADamagedHistoryNeverBreaksThePage(t *testing.T) {
	a := driftApp(t)
	if err := os.WriteFile(a.applied.Path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if counts := a.driftCounts(map[string][]string{"base": {"git"}}, inst("git", "1")); len(counts) != 0 {
		t.Fatalf("a damaged history shows nothing: %v", counts)
	}
	a.installed = inst("git", "1")
	_, err := a.ProfileDrift("base")
	wantErr(t, err, "damaged")
	wantErr(t, a.AcceptProfileState("base"), "damaged")
	if b, _ := os.ReadFile(a.applied.Path); string(b) != "{nope" {
		t.Fatal("the damaged file must be left as it is")
	}
}

func TestWithoutAHistoryStoreNothingBreaks(t *testing.T) {
	a := testApp(t) // a.applied is nil: the settings folder was unavailable
	a.baselineFor = "base"
	a.rememberBaseline(inst("git", "2")) // quietly nothing
	if counts := a.driftCounts(map[string][]string{"base": {"git"}}, inst()); len(counts) != 0 {
		t.Fatal("no store, no counts")
	}
	_, err := a.ProfileDrift("base")
	wantErr(t, err, "settings folder")
	wantErr(t, a.AcceptProfileState("base"), "settings folder")
}
