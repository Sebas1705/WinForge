package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

var day0 = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

// updatesApp has Git and 7-Zip (which needs admin) waiting for an update, a store for the choices
// and a clock that tests move by hand.
func updatesApp(t *testing.T) (*App, *time.Time) {
	t.Helper()
	a := testApp(t)
	a.ignored = &updates.Store{Path: filepath.Join(t.TempDir(), "ignored-updates.json")}
	clock := day0
	a.now = func() time.Time { return clock }
	a.upgrades = []system.Upgrade{
		{ID: "Git.Git", Current: "1", Available: "2"},
		{ID: "7zip.7zip", Current: "23", Available: "24"},
	}
	return a, &clock
}

func stepIDs(t *testing.T, a *App, ids ...string) []string {
	t.Helper()
	plan, err := a.PlanUpgrades(ids)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, s := range plan.Steps {
		out = append(out, s.ID)
	}
	return out
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	m := map[string]bool{}
	for _, g := range got {
		m[g] = true
	}
	for _, w := range want {
		if !m[w] {
			return false
		}
	}
	return true
}

func TestAnIgnoredAppIsLeftOutOfUpdateAll(t *testing.T) {
	a, _ := updatesApp(t)
	if got := stepIDs(t, a); !sameSet(got, []string{"git", "7zip"}) {
		t.Fatalf("nothing ignored yet: %v", got)
	}
	if err := a.IgnoreUpdate("git", "always", 0); err != nil {
		t.Fatal(err)
	}
	if got := stepIDs(t, a); !sameSet(got, []string{"7zip"}) {
		t.Fatalf("an ignored app must not be in update-all: %v", got)
	}
	// Asking for it by name does not bring it back: the person said not to.
	if got := stepIDs(t, a, "git"); len(got) != 0 {
		t.Fatalf("an explicit request must not override the choice: %v", got)
	}
	if err := a.StopIgnoringUpdate("git"); err != nil {
		t.Fatal(err)
	}
	if got := stepIDs(t, a); !sameSet(got, []string{"git", "7zip"}) {
		t.Fatalf("after stopping: %v", got)
	}
}

func TestNeedsAdminFollowsTheStepsThatRemain(t *testing.T) {
	a, _ := updatesApp(t)
	plan, _ := a.PlanUpgrades(nil)
	if !plan.NeedsAdmin {
		t.Fatal("7-Zip needs admin")
	}
	if err := a.IgnoreUpdate("7zip", "always", 0); err != nil {
		t.Fatal(err)
	}
	plan, _ = a.PlanUpgrades(nil)
	if plan.NeedsAdmin {
		t.Fatal("without the app that needs admin, the plan must not ask for it")
	}
}

func TestSkippingAVersionHidesOnlyThatVersion(t *testing.T) {
	a, _ := updatesApp(t)
	if err := a.IgnoreUpdate("git", "version", 0); err != nil {
		t.Fatal(err)
	}
	if got := stepIDs(t, a); !sameSet(got, []string{"7zip"}) {
		t.Fatalf("version 2 is skipped: %v", got)
	}
	a.upgrades[0].Available = "3" // a newer release arrives
	if got := stepIDs(t, a); !sameSet(got, []string{"git", "7zip"}) {
		t.Fatalf("a newer version must show again: %v", got)
	}
}

func TestSkippingAVersionNeedsAnUpdateToSkip(t *testing.T) {
	a, _ := updatesApp(t)
	wantErr(t, a.IgnoreUpdate("vlc", "version", 0), "check for updates first")
}

func TestAPauseEndsOnItsOwn(t *testing.T) {
	a, clock := updatesApp(t)
	if err := a.IgnoreUpdate("git", "until", 7); err != nil {
		t.Fatal(err)
	}
	*clock = day0.AddDate(0, 0, 6)
	if got := stepIDs(t, a); !sameSet(got, []string{"7zip"}) {
		t.Fatalf("still paused on day 6: %v", got)
	}
	*clock = day0.AddDate(0, 0, 7)
	if got := stepIDs(t, a); !sameSet(got, []string{"git", "7zip"}) {
		t.Fatalf("the pause is over on day 7: %v", got)
	}
}

func TestIgnoreUpdateRefusesBadRequests(t *testing.T) {
	a, _ := updatesApp(t)
	wantErr(t, a.IgnoreUpdate("not-an-app", "always", 0), "not a catalog app")
	wantErr(t, a.IgnoreUpdate("git", "forever", 0), "unknown way")
	wantErr(t, a.IgnoreUpdate("git", "until", 0), "between 1 and 365")
	wantErr(t, a.IgnoreUpdate("git", "until", 366), "between 1 and 365")
	wantErr(t, a.IgnoreUpdate("git", "until", -3), "between 1 and 365")
	if _, err := os.Stat(a.ignored.Path); !os.IsNotExist(err) {
		t.Fatal("a refused request must not write anything")
	}
}

func TestWithoutAStoreIgnoringFailsClearly(t *testing.T) {
	a := testApp(t) // no store: the settings folder was unavailable
	wantErr(t, a.IgnoreUpdate("git", "always", 0), "settings folder")
	wantErr(t, a.StopIgnoringUpdate("git"), "settings folder")
	a.upgrades = []system.Upgrade{{ID: "Git.Git", Current: "1", Available: "2"}}
	if plan, err := a.PlanUpgrades(nil); err != nil || len(plan.Steps) != 1 {
		t.Fatalf("with no store there is nothing to ignore: %+v %v", plan, err)
	}
}

func TestADamagedListStopsUpdatesInsteadOfUpdatingEverything(t *testing.T) {
	a, _ := updatesApp(t)
	if err := os.WriteFile(a.ignored.Path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PlanUpgrades(nil); err == nil {
		t.Fatal("update-all must refuse when the ignored list cannot be read")
	}
	wantErr(t, a.ApplyUpgrades(nil), "damaged")
	if a.running {
		t.Fatal("nothing must have started")
	}
}

func TestEverythingIgnoredMeansNothingToUpdate(t *testing.T) {
	a, _ := updatesApp(t)
	for _, id := range []string{"git", "7zip"} {
		if err := a.IgnoreUpdate(id, "always", 0); err != nil {
			t.Fatal(err)
		}
	}
	wantErr(t, a.ApplyUpgrades(nil), "nothing to update")
}
