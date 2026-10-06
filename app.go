package main

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/system"
)

// App is the object bound to the frontend. Every exported method is callable
// from TypeScript; it stays thin - the logic lives in internal/.
type App struct {
	ctx   context.Context
	cat   *catalog.Catalog
	store *profile.Store

	mu            sync.Mutex
	installed     map[string]system.Installed
	upgrades      []system.Upgrade
	healthReport  *health.Report
	healthUpdates *health.UpdateScan
	running       bool
	cancel        context.CancelFunc
}

func NewApp() *App {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		// The catalog is embedded and covered by tests; failing here means a
		// broken build, not a user error.
		panic(err)
	}
	store, err := profile.DefaultStore()
	if err != nil {
		panic(err)
	}
	return &App{cat: cat, store: store}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// scanStep tells the UI which stage of a scan ("pc" or "health") just began.
func (a *App) scanStep(scan, step string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "scan:step", map[string]string{"scan": scan, "step": step})
	}
}

// AppInfo is a catalog app as the UI needs it.
type AppInfo struct {
	catalog.App
	Installed bool     `json:"installed"`
	Version   string   `json:"version,omitempty"`
	Sources   []string `json:"sources,omitempty"`
}

// State is everything the UI renders.
type State struct {
	Version  string        `json:"version"`
	Admin    bool          `json:"admin"`
	Apps     []AppInfo     `json:"apps"`
	Profiles []ProfileInfo `json:"profiles"`
	// Featured lists popular catalog ids in display order.
	Featured []string `json:"featured"`
	// WingetError is set when winget is missing; the app still works for
	// detection but cannot install.
	WingetError string `json:"wingetError,omitempty"`
}

// ProfileInfo is a profile plus its flattened contents.
type ProfileInfo struct {
	catalog.Profile
	Builtin         bool     `json:"builtin"`
	Resolved        []string `json:"resolved"`
	ResolvedRecipes []string `json:"resolvedRecipes"`
}

// GetState rescans the PC and returns the full UI state.
func (a *App) GetState() (State, error) {
	inv, err := system.SnapshotWith(a.ctx, func(s string) { a.scanStep("pc", s) })
	a.scanStep("pc", "match")
	st := State{Version: version, Admin: system.IsElevated(), Apps: []AppInfo{}, Profiles: []ProfileInfo{}, Featured: append([]string{}, a.cat.Featured...)}
	if err != nil {
		st.WingetError = err.Error()
	}
	installed := system.Detect(a.cat, inv)
	a.mu.Lock()
	a.installed = installed
	a.mu.Unlock()

	for _, id := range sortedApps(a.cat) {
		app := a.cat.Apps[id]
		info := AppInfo{App: *app}
		if i, ok := installed[id]; ok {
			info.Installed, info.Version, info.Sources = true, i.Version, i.Sources
		}
		st.Apps = append(st.Apps, info)
	}
	profiles := a.cat.Profiles
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		st.Profiles = append(st.Profiles, a.profileInfo(*profiles[id], true))
	}
	user, err := a.store.List(a.cat)
	if err != nil {
		return st, err
	}
	for _, p := range user {
		st.Profiles = append(st.Profiles, a.profileInfo(p, false))
	}
	return st, nil
}

func (a *App) profileInfo(p catalog.Profile, builtin bool) ProfileInfo {
	pi := ProfileInfo{Profile: p, Builtin: builtin, Resolved: []string{}, ResolvedRecipes: []string{}}
	if res, err := a.cat.ResolveProfile(&p); err == nil {
		for _, pa := range res.Apps {
			pi.Resolved = append(pi.Resolved, pa.ID)
		}
		pi.ResolvedRecipes = res.Recipes
	}
	return pi
}

func sortedApps(c *catalog.Catalog) []string {
	ids := make([]string, 0, len(c.Apps))
	for id := range c.Apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Plan computes what applying a profile would do on this PC. The profile may
// be an unsaved selection built in the UI.
func (a *App) Plan(p catalog.Profile) (install.Plan, error) {
	res, err := a.cat.ResolveProfile(&p)
	if err != nil {
		return install.Plan{}, err
	}
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	return install.BuildPlan(a.cat, res, installed), nil
}

// Apply runs the plan in the background, streaming "install" events and a
// final "install:done".
func (a *App) Apply(p catalog.Profile) error {
	plan, err := a.Plan(p)
	if err != nil {
		return err
	}
	return a.start(plan)
}

// start runs a plan in the background; only one runs at a time.
func (a *App) start(plan install.Plan) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("an installation is already running")
	}
	a.running = true
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.mu.Unlock()

	go func() {
		failed := install.Run(ctx, a.cat, plan, install.OSExecutor{}, func(e install.Event) {
			runtime.EventsEmit(a.ctx, "install", e)
		})
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		cancel()
		runtime.EventsEmit(a.ctx, "install:done", failed)
	}()
	return nil
}

// Cancel stops the running installation after the current process is killed.
func (a *App) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// SaveProfile stores a user profile.
func (a *App) SaveProfile(p catalog.Profile) error {
	if a.cat.Profiles[p.ID] != nil {
		return errors.New("that id belongs to a built-in profile")
	}
	if _, err := a.cat.ResolveProfile(&p); err != nil {
		return err
	}
	p.Kind = "custom"
	return a.store.Save(p)
}

// DeleteProfile removes a user profile.
func (a *App) DeleteProfile(id string) error { return a.store.Delete(id) }

// ProfileFromPC builds a profile out of the catalog apps installed right now.
func (a *App) ProfileFromPC(id, name string, pinVersions bool) catalog.Profile {
	a.mu.Lock()
	defer a.mu.Unlock()
	versions := make(map[string]string, len(a.installed))
	for appID, i := range a.installed {
		versions[appID] = i.Version
	}
	return profile.FromInstalled(id, name, versions, pinVersions)
}

// ExportProfile asks where to save and writes the shareable profile file.
// It returns the chosen path, or "" if the dialog was cancelled.
func (a *App) ExportProfile(p catalog.Profile) (string, error) {
	return a.saveWith(p.ID+".winforge.json", "WinForge profile (*.json)", "*.json", func() ([]byte, error) {
		return profile.Export(p)
	})
}

// ExportWinget writes a file `winget import` understands.
func (a *App) ExportWinget(p catalog.Profile) (string, error) {
	res, err := a.cat.ResolveProfile(&p)
	if err != nil {
		return "", err
	}
	return a.saveWith(p.ID+".winget.json", "winget import file (*.json)", "*.json", func() ([]byte, error) {
		return profile.WingetImport(res.Apps, a.cat)
	})
}

func (a *App) saveWith(name, label, pattern string, render func() ([]byte, error)) (string, error) {
	b, err := render()
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: label, Pattern: pattern}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, b, 0o644)
}

// ImportResult is what the UI shows before the user confirms an import.
type ImportResult struct {
	Profile        catalog.Profile `json:"profile"`
	UnknownApps    []string        `json:"unknownApps"`
	UnknownRecipes []string        `json:"unknownRecipes"`
}

// ImportProfile asks for a file and validates it against the catalog. It does
// not save anything; the UI calls SaveProfile after the user reviews it.
func (a *App) ImportProfile() (*ImportResult, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "WinForge profile (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	imp, err := profile.Import(b, a.cat)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Profile: imp.Profile, UnknownApps: imp.UnknownApps, UnknownRecipes: imp.UnknownRecipes}, nil
}

// RestartAsAdmin relaunches through UAC and quits this instance.
func (a *App) RestartAsAdmin() error {
	if err := system.RestartElevated(); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// OpenURL opens an https link in the default browser.
func (a *App) OpenURL(u string) {
	if len(u) > 8 && u[:8] == "https://" {
		runtime.BrowserOpenURL(a.ctx, u)
	}
}

// UpgradeInfo is an installed catalog app with a newer version available.
type UpgradeInfo struct {
	ID        string `json:"id"` // catalog id
	Name      string `json:"name"`
	Publisher string `json:"publisher"`
	Current   string `json:"current"`
	Available string `json:"available"`
}

// Upgrades asks winget which installed packages have updates and keeps those
// the catalog knows; everything else on the PC is left alone.
func (a *App) Upgrades() ([]UpgradeInfo, error) {
	ups, err := system.WingetUpgrades(a.ctx)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.upgrades = ups
	a.mu.Unlock()
	out := []UpgradeInfo{}
	for _, s := range install.UpgradePlan(a.cat, ups, nil).Steps {
		app := a.cat.Apps[s.ID]
		cur := ""
		for _, u := range ups {
			if u.ID == app.Winget {
				cur = u.Current
			}
		}
		out = append(out, UpgradeInfo{ID: s.ID, Name: app.Name, Publisher: app.Publisher, Current: cur, Available: s.Version})
	}
	return out, nil
}

// PlanUpgrades previews updating the given catalog apps (all when empty).
func (a *App) PlanUpgrades(ids []string) install.Plan {
	a.mu.Lock()
	ups := a.upgrades
	a.mu.Unlock()
	return install.UpgradePlan(a.cat, ups, ids)
}

// ApplyUpgrades updates the given catalog apps in the background.
func (a *App) ApplyUpgrades(ids []string) error {
	plan := a.PlanUpgrades(ids)
	if len(plan.Steps) == 0 {
		return errors.New("nothing to update")
	}
	return a.start(plan)
}

// ExportScript writes a PowerShell script for what applying the profile would
// do on this PC. It returns the chosen path, or "" if the dialog was cancelled.
func (a *App) ExportScript(p catalog.Profile) (string, error) {
	plan, err := a.Plan(p)
	if err != nil {
		return "", err
	}
	return a.saveWith(p.ID+".ps1", "PowerShell script (*.ps1)", "*.ps1", func() ([]byte, error) {
		return []byte(install.Script(a.cat, plan, p.Name)), nil
	})
}
