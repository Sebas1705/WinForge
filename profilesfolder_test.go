package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/appconfig"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

// folderApp has a default profiles folder with two profiles in it, a config store and no custom folder.
func folderApp(t *testing.T) (a *App, defaultDir string) {
	t.Helper()
	a = testApp(t)
	defaultDir = a.store.Dir
	a.defaultStore = func() (*profile.Store, error) { return &profile.Store{Dir: defaultDir}, nil }
	a.config = &appconfig.Store{Path: filepath.Join(t.TempDir(), "config.json")}
	for _, p := range []catalog.Profile{{ID: "alpha", Name: "Alpha", Apps: []catalog.ProfileApp{{ID: "git"}}}, {ID: "beta", Name: "Beta", Apps: []catalog.ProfileApp{{ID: "vlc"}}}} {
		if err := a.SaveProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	return a, defaultDir
}

// names lists the user profiles the app would show, without scanning the PC the way GetState does.
func names(t *testing.T, a *App) []string {
	t.Helper()
	list, err := a.profileStore().List(a.cat)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, p := range list {
		out = append(out, p.ID)
	}
	return out
}

func TestFolderStartsAsTheDefault(t *testing.T) {
	a, def := folderApp(t)
	info := a.ProfilesFolder()
	if info.Path != def || info.Default != def || info.Custom || info.Count != 2 || info.Problem != "" {
		t.Fatalf("%+v", info)
	}
}

func TestMovingTheFolderCopiesTheProfilesAndKeepsWorkingThere(t *testing.T) {
	a, def := folderApp(t)
	sync := t.TempDir() // stands for a folder inside OneDrive
	change, err := a.SetProfilesFolder(sync, true)
	if err != nil || change.Copied != 2 || change.Skipped != 0 || change.Info.Path != sync || !change.Info.Custom || change.Info.Count != 2 {
		t.Fatalf("%+v %v", change, err)
	}
	if got := names(t, a); len(got) != 2 {
		t.Fatalf("the profiles are still listed: %v", got)
	}
	// a profile saved now lands in the new folder, not in the old one
	if err := a.SaveProfile(catalog.Profile{ID: "gamma", Name: "Gamma", Apps: []catalog.ProfileApp{{ID: "git"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sync, "gamma.json")); err != nil {
		t.Fatalf("saved in the new folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(def, "gamma.json")); err == nil {
		t.Fatal("must not be written to the old folder any more")
	}
	// the old copies are left where they were
	for _, f := range []string{"alpha.json", "beta.json"} {
		if _, err := os.Stat(filepath.Join(def, f)); err != nil {
			t.Errorf("the old folder must be left alone: %v", err)
		}
	}
	// and the choice survives a restart
	again := &App{cat: a.cat, config: a.config, defaultStore: a.defaultStore, store: &profile.Store{Dir: def}}
	again.useConfiguredProfiles()
	if got := again.ProfilesFolder(); got.Path != sync || !got.Custom {
		t.Fatalf("after restart: %+v", got)
	}
}

func TestMovingWithoutCopyingStartsFromWhatIsThere(t *testing.T) {
	a, _ := folderApp(t)
	other := t.TempDir()
	if err := (&profile.Store{Dir: other}).Save(profile.FromInstalled("delta", "Delta", map[string]string{"git": ""}, false)); err != nil {
		t.Fatal(err)
	}
	change, err := a.SetProfilesFolder(other, false)
	if err != nil || change.Copied != 0 {
		t.Fatalf("%+v %v", change, err)
	}
	if got := names(t, a); len(got) != 1 || got[0] != "delta" {
		t.Fatalf("only the new folder's profiles: %v", got)
	}
}

func TestACopyNeverOverwritesWhatTheOtherFolderHas(t *testing.T) {
	a, _ := folderApp(t)
	other := t.TempDir()
	if err := (&profile.Store{Dir: other}).Save(profile.FromInstalled("alpha", "Alpha from another PC", map[string]string{"vlc": ""}, false)); err != nil {
		t.Fatal(err)
	}
	change, err := a.SetProfilesFolder(other, true)
	if err != nil || change.Copied != 1 || change.Skipped != 1 {
		t.Fatalf("%+v %v", change, err)
	}
	b, _ := os.ReadFile(filepath.Join(other, "alpha.json"))
	if !strings.Contains(string(b), "Alpha from another PC") {
		t.Fatal("the other PC's profile was overwritten")
	}
}

func TestBadFoldersAreRefusedAndNothingChanges(t *testing.T) {
	a, def := folderApp(t)
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{
		"missing":  filepath.Join(t.TempDir(), "nope"),
		"a file":   file,
		"relative": "profiles",
		"empty":    "",
	} {
		if _, err := a.SetProfilesFolder(dir, true); err == nil {
			t.Errorf("%s (%q) must be refused", name, dir)
		}
	}
	if _, err := a.SetProfilesFolder(def, true); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("the current folder is not a change: %v", err)
	}
	if info := a.ProfilesFolder(); info.Path != def || info.Custom {
		t.Fatalf("a refused change must change nothing: %+v", info)
	}
	if _, err := os.Stat(a.config.Path); !os.IsNotExist(err) {
		t.Fatal("nothing should have been saved")
	}
}

func TestChoosingTheDefaultFolderIsGoingBackToTheDefault(t *testing.T) {
	a, def := folderApp(t)
	sync := t.TempDir()
	if _, err := a.SetProfilesFolder(sync, false); err != nil {
		t.Fatal(err)
	}
	change, err := a.SetProfilesFolder(def, false)
	if err != nil || change.Info.Custom {
		t.Fatalf("%+v %v", change, err)
	}
	if _, err := os.Stat(a.config.Path); !os.IsNotExist(err) {
		t.Fatal("the default needs no saved choice")
	}
}

func TestResetGoesBackWithoutTouchingFiles(t *testing.T) {
	a, def := folderApp(t)
	sync := t.TempDir()
	if _, err := a.SetProfilesFolder(sync, true); err != nil {
		t.Fatal(err)
	}
	info, err := a.ResetProfilesFolder()
	if err != nil || info.Path != def || info.Custom || info.Count != 2 {
		t.Fatalf("%+v %v", info, err)
	}
	if list, _ := (&profile.Store{Dir: sync}).List(a.cat); len(list) != 2 {
		t.Fatal("the synced folder must keep its profiles")
	}
}

func TestAnUnavailableFolderAtStartDoesNotBreakTheApp(t *testing.T) {
	a, def := folderApp(t)
	gone := filepath.Join(t.TempDir(), "OneDrive-not-mounted")
	if err := a.config.Save(appconfig.Config{ProfilesDir: gone}); err != nil {
		t.Fatal(err)
	}
	a.useConfiguredProfiles()
	info := a.ProfilesFolder()
	if info.Path != def || info.Custom || !strings.Contains(info.Problem, "not available") || !strings.Contains(info.Problem, gone) {
		t.Fatalf("the default is used and the person is told: %+v", info)
	}
	if !strings.Contains(a.StartupError(), "not available") {
		t.Fatalf("the problem reaches the interface: %q", a.StartupError())
	}
	if got := names(t, a); len(got) != 2 {
		t.Fatalf("the profiles are still reachable: %v", got)
	}
	// and the choice is kept, so the folder is used again as soon as it is back
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	a.useConfiguredProfiles()
	if got := a.ProfilesFolder(); got.Path != gone || !got.Custom {
		t.Fatalf("%+v", got)
	}
}

func TestADamagedConfigFallsBackAndSaysWhere(t *testing.T) {
	a, def := folderApp(t)
	if err := os.WriteFile(a.config.Path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.useConfiguredProfiles()
	info := a.ProfilesFolder()
	if info.Path != def || !strings.Contains(info.Problem, "damaged") {
		t.Fatalf("%+v", info)
	}
	if b, _ := os.ReadFile(a.config.Path); string(b) != "{bad" {
		t.Fatal("a damaged config must not be overwritten at start")
	}
}

func TestWithoutAConfigStoreTheFolderCannotChange(t *testing.T) {
	a := testApp(t)
	if _, err := a.SetProfilesFolder(t.TempDir(), true); err == nil || !strings.Contains(err.Error(), "settings folder") {
		t.Fatalf("%v", err)
	}
	if _, err := a.ResetProfilesFolder(); err == nil {
		t.Fatal("reset needs the config store too")
	}
}
