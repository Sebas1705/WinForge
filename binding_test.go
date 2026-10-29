package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/system"
)

// testApp is an App over a tiny catalog and stores in a temp dir, so the
// binding layer can be driven without a window, winget or the user's profile.
func testApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cat := &catalog.Catalog{
		Apps: map[string]*catalog.App{
			"git":  {ID: "git", Name: "Git", Winget: "Git.Git"},
			"vlc":  {ID: "vlc", Name: "VLC", Winget: "VideoLAN.VLC"},
			"7zip": {ID: "7zip", Name: "7-Zip", Winget: "7zip.7zip", Admin: true},
		},
		Profiles: map[string]*catalog.Profile{"base": {ID: "base", Name: "Base"}},
		Recipes: map[string]*catalog.Recipe{
			"repair-sfc":           {ID: "repair-sfc", Description: "sfc"},
			"repair-restore-point": {ID: "repair-restore-point", Description: "restore point"},
			"repair-windows-files": {ID: "repair-windows-files", Description: "files"},
			"tweak-dark":           {ID: "tweak-dark", Description: "not a repair"},
		},
		Emulation: &catalog.Emulation{
			Emulators: map[string][]string{"mgba": {"gba"}},
			Run: map[string]catalog.RunSpec{
				"mgba":  {Args: []string{"{file}"}},
				"other": {},
			},
		},
		Games: []catalog.Game{
			{ID: "frontend-game", Name: "Frontend", System: "gba"},
			{ID: "nes-game", Name: "NES", System: "nes"},
		},
	}
	return &App{
		ctx:     context.Background(),
		cat:     cat,
		store:   &profile.Store{Dir: filepath.Join(dir, "profiles")},
		pending: &install.PendingStore{Path: filepath.Join(dir, "pending.json")},
	}
}

func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", sub)
	}
	if !strings.Contains(err.Error(), sub) {
		t.Fatalf("error = %q, want it to contain %q", err, sub)
	}
}

func TestSortedApps(t *testing.T) {
	a := testApp(t)
	got := sortedApps(a.cat)
	if len(got) != 3 {
		t.Fatalf("sortedApps = %v", got)
	}
	again := sortedApps(a.cat)
	for i := range got {
		if got[i] != again[i] {
			t.Errorf("order is not deterministic: %v vs %v", got, again)
		}
	}
}

func TestPlanValidation(t *testing.T) {
	a := testApp(t)
	_, err := a.Plan(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "ghost"}}})
	wantErr(t, err, "unknown app")

	a.installed = map[string]system.Installed{"git": {}}
	plan, err := a.Plan(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "git"}, {ID: "vlc"}, {ID: "7zip"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %+v, want vlc and 7zip", plan.Steps)
	}
	if !plan.NeedsAdmin {
		t.Error("an admin app in the plan must set NeedsAdmin")
	}
	if len(plan.AlreadyInstalled) != 1 || plan.AlreadyInstalled[0] != "git" {
		t.Errorf("AlreadyInstalled = %v", plan.AlreadyInstalled)
	}
}

func TestApplyErrorMapping(t *testing.T) {
	a := testApp(t)
	wantErr(t, a.Apply(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "ghost"}}}), "unknown app")

	// A second run is refused while one is going; nothing is started.
	a.running = true
	wantErr(t, a.Apply(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "vlc"}}}), "already running")
	if p := a.pending.Load(); p != nil {
		t.Errorf("a refused run must not store a pending run, got %+v", p)
	}
}

func TestApplyEntryPointsRejectEmptyWork(t *testing.T) {
	a := testApp(t)
	wantErr(t, a.ApplyUninstall(nil), "nothing to remove")
	wantErr(t, a.ApplyUninstall([]string{"ghost"}), "nothing to remove")
	wantErr(t, a.ApplyUpgrades(nil), "nothing to update")
	wantErr(t, a.ApplyRepairs(nil), "nothing to repair")
	wantErr(t, a.ApplyRepairs([]string{"tweak-dark"}), "is not a repair")
	wantErr(t, a.ApplyRepairs([]string{"repair-ghost"}), "is not a repair")

	a.running = true
	wantErr(t, a.ApplyUninstall([]string{"git"}), "already running")
	wantErr(t, a.ApplyRepairs([]string{"repair-sfc"}), "already running")
}

func TestPlanRepairsOnlyAcceptsRepairs(t *testing.T) {
	a := testApp(t)
	_, err := a.PlanRepairs([]string{"tweak-dark"})
	wantErr(t, err, "is not a repair")
	plan, err := a.PlanRepairs([]string{"repair-windows-files", "repair-windows-files"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].ID != "repair-restore-point" {
		t.Errorf("risky repair should be preceded by one restore point: %+v", plan.Steps)
	}
}

func TestPlanUninstallSkipsUnknownIDs(t *testing.T) {
	a := testApp(t)
	plan := a.PlanUninstall([]string{"git", "ghost", "7zip"})
	if len(plan.Steps) != 2 || !plan.NeedsAdmin {
		t.Errorf("plan = %+v", plan)
	}
}

func TestSaveAndDeleteProfile(t *testing.T) {
	a := testApp(t)
	wantErr(t, a.SaveProfile(catalog.Profile{ID: "base", Name: "Mine"}), "built-in")
	wantErr(t, a.SaveProfile(catalog.Profile{ID: "mine", Apps: []catalog.ProfileApp{{ID: "ghost"}}}), "unknown app")
	wantErr(t, a.SaveProfile(catalog.Profile{ID: "../evil", Name: "x"}), "invalid profile id")

	if err := a.SaveProfile(catalog.Profile{ID: "mine", Name: "Mine", Kind: "base", Apps: []catalog.ProfileApp{{ID: "git"}}}); err != nil {
		t.Fatal(err)
	}
	list, err := a.store.List(a.cat)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if list[0].Kind != "custom" {
		t.Errorf("Kind = %q: user profiles are always custom, whatever the UI sent", list[0].Kind)
	}
	if err := a.DeleteProfile("mine"); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteProfile("mine"); err != nil {
		t.Errorf("deleting a missing profile is not an error, got %v", err)
	}
	wantErr(t, a.DeleteProfile("../evil"), "invalid profile id")
}

func TestProfileFromPC(t *testing.T) {
	a := testApp(t)
	a.installed = map[string]system.Installed{"git": {Version: "2.1"}, "vlc": {Version: "3.0"}}
	p := a.ProfileFromPC("pc", "My PC", true)
	if p.ID != "pc" || p.Name != "My PC" || len(p.Apps) != 2 {
		t.Fatalf("profile = %+v", p)
	}
	for _, pa := range p.Apps {
		if pa.Version == "" {
			t.Errorf("pinVersions=true should keep the version of %s", pa.ID)
		}
	}
	for _, pa := range a.ProfileFromPC("pc", "My PC", false).Apps {
		if pa.Version != "" {
			t.Errorf("pinVersions=false must not pin %s", pa.ID)
		}
	}
}

func TestExportsRejectUnknownApps(t *testing.T) {
	a := testApp(t)
	bad := catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "ghost"}}}
	_, err := a.ExportWinget(bad)
	wantErr(t, err, "unknown app")
	_, err = a.ExportScript(bad)
	wantErr(t, err, "unknown app")
}

func TestShareCodeRoundTripAndErrors(t *testing.T) {
	a := testApp(t)
	_, err := a.ShareCode(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "ghost"}}})
	wantErr(t, err, "unknown app")

	code, err := a.ShareCode(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git"}, {ID: "vlc"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(code, "\r\n") {
		t.Errorf("a share code is one line, got %q", code)
	}
	res, err := a.ImportCode(code)
	if err != nil {
		t.Fatal(err)
	}
	if res.Profile.Name != "Mine" || len(res.Profile.Apps) != 2 || len(res.UnknownApps) != 0 {
		t.Errorf("round trip = %+v", res)
	}
	if _, err := a.ImportCode("this is not a code"); err == nil {
		t.Error("garbage must not import")
	}
	if _, err := a.ImportCode(""); err == nil {
		t.Error("an empty code must not import")
	}
}

func TestImportCodeReportsUnknownApps(t *testing.T) {
	sender := testApp(t)
	sender.cat.Apps["extra"] = &catalog.App{ID: "extra", Name: "Extra", Winget: "X.Y"}
	code, err := sender.ShareCode(catalog.Profile{ID: "p", Name: "P", Apps: []catalog.ProfileApp{{ID: "git"}, {ID: "extra"}}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := testApp(t).ImportCode(code)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.UnknownApps) != 1 || res.UnknownApps[0] != "extra" {
		t.Errorf("UnknownApps = %v, want [extra]", res.UnknownApps)
	}
}

func TestPendingFlow(t *testing.T) {
	a := testApp(t)
	wantErr(t, a.ResumePending(), "nothing to continue")

	steps := []install.Step{{Kind: install.StepApp, ID: "vlc", Name: "VLC"}, {Kind: install.StepApp, ID: "ghost"}}
	if err := a.pending.Save(install.Pending{Title: "t", Steps: steps}); err != nil {
		t.Fatal(err)
	}

	// Steps for apps the catalog no longer has are dropped; the rest is kept.
	plan := a.PlanPending()
	if len(plan.Steps) != 1 || plan.Steps[0].ID != "vlc" {
		t.Fatalf("PlanPending = %+v", plan)
	}
	// An app that is meanwhile installed no longer needs its step.
	a.installed = map[string]system.Installed{"vlc": {}}
	if got := a.PlanPending(); len(got.Steps) != 0 {
		t.Errorf("installed app should leave the plan, got %+v", got)
	}
	if a.pending.Load() != nil {
		t.Error("an empty pending run is forgotten")
	}

	a.installed = nil
	if err := a.pending.Save(install.Pending{Title: "t", Steps: steps[:1]}); err != nil {
		t.Fatal(err)
	}
	if err := a.DiscardPending(); err != nil {
		t.Fatal(err)
	}
	if a.pending.Load() != nil {
		t.Error("DiscardPending should forget the run")
	}
}

func TestResumePendingRefusesSecondRun(t *testing.T) {
	a := testApp(t)
	if err := a.pending.Save(install.Pending{Title: "t", Steps: []install.Step{{Kind: install.StepApp, ID: "vlc"}}}); err != nil {
		t.Fatal(err)
	}
	a.running = true
	wantErr(t, a.ResumePending(), "already running")
	if a.pending.Load() == nil {
		t.Error("a refused resume must keep the stored run")
	}
}

func TestPlanUpgradesUsesLastScan(t *testing.T) {
	a := testApp(t)
	if plan := a.PlanUpgrades(nil); len(plan.Steps) != 0 {
		t.Errorf("no scan yet: want no steps, got %+v", plan.Steps)
	}
	a.upgrades = []system.Upgrade{{ID: "Git.Git", Current: "1", Available: "2"}}
	plan := a.PlanUpgrades(nil)
	if len(plan.Steps) != 1 || plan.Steps[0].ID != "git" {
		t.Errorf("plan = %+v", plan.Steps)
	}
	if plan := a.PlanUpgrades([]string{"vlc"}); len(plan.Steps) != 0 {
		t.Errorf("a filter for an app without an update must yield nothing, got %+v", plan.Steps)
	}
	a.running = true
	wantErr(t, a.ApplyUpgrades([]string{"git"}), "already running")
}

func TestCancel(t *testing.T) {
	a := testApp(t)
	a.Cancel() // no run: harmless
	called := false
	a.cancel = func() { called = true }
	a.Cancel()
	if !called {
		t.Error("Cancel should call the run's cancel func")
	}
}

func TestOpenURLIgnoresNonHTTPS(t *testing.T) {
	a := &App{} // a nil ctx would panic if the browser were asked to open anything
	for _, u := range []string{"", "http://example.com", "javascript:alert(1)", "file:///c:/windows", "https:/", "https://", "ftp://x.y/z"} {
		a.OpenURL(u)
	}
}

func TestRestoreRequiresPickedBackup(t *testing.T) {
	a := testApp(t)
	_, err := a.RestoreSettings([]string{"x"})
	wantErr(t, err, "choose a backup first")
	_, err = a.RestoreFolders([]string{"x"}, t.TempDir())
	wantErr(t, err, "choose a backup first")

	a.restorePath = filepath.Join(t.TempDir(), "missing.zip")
	_, err = a.RestoreSettings(nil)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a vanished backup should surface the file error, got %v", err)
	}

	f := filepath.Join(t.TempDir(), "b.zip")
	if err := os.WriteFile(f, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.restorePath = f
	_, err = a.RestoreFolders([]string{"x"}, "   ")
	wantErr(t, err, "choose a folder to restore into")
	if _, err = a.RestoreSettings([]string{"x"}); err == nil {
		t.Error("a file that is not a zip must fail to restore")
	}
}

func TestGameLookup(t *testing.T) {
	a := testApp(t)
	if g, ok := a.game("nes-game"); !ok || g.Name != "NES" {
		t.Errorf("game(nes-game) = %+v, %v", g, ok)
	}
	if _, ok := a.game("ghost"); ok {
		t.Error("unknown game must not be found")
	}
}

func TestPlays(t *testing.T) {
	e := &catalog.Emulation{Emulators: map[string][]string{"mgba": {"gba", "gb"}}}
	cases := []struct {
		emu, sys string
		want     bool
	}{{"mgba", "gba", true}, {"mgba", "gb", true}, {"mgba", "nes", false}, {"ghost", "gba", false}}
	for _, c := range cases {
		if got := plays(e, c.emu, c.sys); got != c.want {
			t.Errorf("plays(%s,%s) = %v, want %v", c.emu, c.sys, got, c.want)
		}
	}
}

func TestGameBindingsRejectBadInput(t *testing.T) {
	a := testApp(t)
	_, err := a.InstallGame("ghost")
	wantErr(t, err, "unknown game")
	wantErr(t, a.RemoveGame("ghost"), "unknown game")
	wantErr(t, a.PlayGame("ghost", "mgba"), "unknown game")
	wantErr(t, a.OpenGameFolder("ghost"), "unknown game")

	// The emulator must play the game's system.
	wantErr(t, a.PlayGame("nes-game", "mgba"), "does not play this system")
	wantErr(t, a.PlayGame("frontend-game", "ghost"), "does not play this system")

	// Emulators without a known command line are not launched.
	a.cat.Emulation.Emulators["other"] = []string{"gba"}
	wantErr(t, a.PlayGame("frontend-game", "other"), "does not know how to start")

	// A game already being installed is refused.
	a.installing = map[string]bool{"nes-game": true}
	_, err = a.InstallGame("nes-game")
	wantErr(t, err, "already being installed")
}

func TestPlayGameWithoutEmulationData(t *testing.T) {
	a := testApp(t)
	a.cat.Emulation = nil
	wantErr(t, a.PlayGame("nes-game", "mgba"), "does not play this system")
}
