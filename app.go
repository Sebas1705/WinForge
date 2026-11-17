package main

import (
	"context"
	"errors"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/appconfig"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/drift"
	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/privacy"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// App is the object bound to the frontend. Every exported method is callable
// from TypeScript; it stays thin - the logic lives in internal/.
type App struct {
	ctx   context.Context
	cat   *catalog.Catalog
	store *profile.Store // guarded by mu: the person can move it to another folder while the app runs

	config       *appconfig.Store               // nil when the settings folder is unavailable
	defaultStore func() (*profile.Store, error) // replaced by tests; nil means profile.DefaultStore
	profilesNote string                         // why the chosen profiles folder is not in use, if it is not

	mu            sync.Mutex
	installed     map[string]system.Installed
	upgrades      []system.Upgrade
	healthReport  *health.Report
	healthUpdates *health.UpdateScan
	pending       *install.PendingStore
	applied       *drift.Store     // what the PC looked like when each profile was applied; nil when the settings folder is unavailable
	baselineFor   string           // a profile that was just applied in full: the next scan is its baseline
	ignored       *updates.Store   // updates the person chose not to install; nil when the settings folder is unavailable
	now           func() time.Time // replaced by tests
	history       history.Store    // what was installed, updated and rolled back; nil when the settings folder is unavailable
	historyState  historySession
	detect        func() map[string]system.Installed // replaced by tests; nil means scan this PC
	restorePath   string
	installing    map[string]bool

	// storage remembers what the storage scans listed; emitter and runner are
	// replaced by tests (nil means the real Wails events and real commands).
	storage    storageSession
	env        *storage.Env // replaced by tests; nil means this PC's real folders
	emitter    func(event string, data any)
	runner     storage.Runner
	running    bool
	cancel     context.CancelFunc
	startupErr error

	// The Startup page: what the last list showed, and the machine behind it. startupSys, elevated
	// and reveal are replaced by tests (nil means the real registry, token and Explorer).
	startupState startupSession
	startupSys   startup.System
	elevated     func() bool
	reveal       func(path string) error

	// The Environment page: which fixes the last scan offered, and the machine behind it (envSys is
	// replaced by tests; nil means the real registry).
	envState envSession
	envSys   envdoctor.System
	// The Privacy page (privacybind.go): what it listed, and the registry behind it (replaced by tests).
	privacyState privacySession
	privacySys   privacy.System
	// The Windows features page reads through featuresSys (nil means this PC); tests replace it.
	featuresSys winfeatures.System
	// The Services and Scheduled tasks tabs of the Startup page (servicesbind.go, tasksbind.go).
	svc   svcFeature
	hhist hhistState // PC health history and battery (healthhistorybind.go)
	// The Uninstall page: what this session uninstalled and what the last leftovers search listed.
	uninst uninstallState
	// The Updates page's automatic updates (the scheduled task); replaced by tests.
	sched schedSession
	// The Settings page's diagnostic report (diagbind.go).
	diag diagSession
	// Where the Settings page finds winforge-mcp.exe (assistantbind.go); nil means beside this exe.
	assistantExe func() (string, error)
	// The offline kit: the kit last opened and the task that is running (offlinekitbind.go).
	kit kitState
}

func NewApp() *App {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		// The catalog is embedded and covered by tests; failing here means a
		// broken build, not a user error.
		panic(err)
	}
	app := newApp(cat, profile.DefaultStore, install.DefaultPendingStore)
	if c, err := appconfig.DefaultStore(); err != nil {
		log.Printf("winforge: config: %v", err)
	} else {
		app.config = c
		app.useConfiguredProfiles()
	}
	if d, err := drift.DefaultStore(); err != nil {
		log.Printf("winforge: profile history: %v", err)
	} else {
		app.applied = d
	}
	if s, err := updates.DefaultStore(); err != nil {
		log.Printf("winforge: ignored updates: %v", err)
	} else {
		app.ignored = s
	}
	if h, err := history.DefaultStore(); err != nil {
		log.Printf("winforge: history: %v", err)
	} else {
		app.history = h
	}
	app.openHealthHistory()
	return app
}

// errStorageUnavailable is what profile operations return when the user's
// config folder could not be opened at startup.
var errStorageUnavailable = errors.New("WinForge could not open its settings folder, so profiles cannot be saved")

// newApp builds the App with the given store constructors. A store that cannot
// be opened is a problem with the user's environment (no %AppData%, for
// instance), not a broken build: the app starts without it, logs the problem
// and reports it to the UI through StartupError and State.StartupError.
func newApp(cat *catalog.Catalog, openStore func() (*profile.Store, error), openPending func() (*install.PendingStore, error)) *App {
	a := &App{cat: cat}
	var problems []string
	if store, err := openStore(); err != nil {
		problems = append(problems, "profiles: "+err.Error())
	} else {
		a.store = store
	}
	if pending, err := openPending(); err != nil {
		problems = append(problems, "unfinished runs: "+err.Error())
	} else {
		a.pending = pending
	}
	if len(problems) > 0 {
		a.startupErr = errors.New("Could not open WinForge's settings folder (" + strings.Join(problems, "; ") + "). The app works, but profiles and unfinished runs will not be saved.")
		log.Printf("winforge: startup: %v", a.startupErr)
	}
	return a
}

// StartupError is the problem found while starting, or "" when all is well.
func (a *App) StartupError() string {
	a.mu.Lock()
	note := a.profilesNote
	a.mu.Unlock()
	switch {
	case a.startupErr != nil && note != "":
		return a.startupErr.Error() + " " + note
	case a.startupErr != nil:
		return a.startupErr.Error()
	}
	return note
}

// loadPending, savePending and clearPending tolerate a missing pending store.
func (a *App) loadPending() *install.Pending {
	if a.pending == nil {
		return nil
	}
	return a.pending.Load()
}

func (a *App) savePending(p install.Pending) {
	if a.pending != nil {
		_ = a.pending.Save(p)
	}
}

func (a *App) clearPending() error {
	if a.pending == nil {
		return nil
	}
	return a.pending.Clear()
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
	// Pending is a run that stopped before finishing (failed steps, a restart,
	// a crash) and can be continued.
	Pending *install.Pending `json:"pending,omitempty"`
	// StartupError is set when the settings folder could not be opened.
	StartupError string `json:"startupError,omitempty"`
}

// ProfileInfo is a profile plus its flattened contents.
type ProfileInfo struct {
	catalog.Profile
	Builtin         bool     `json:"builtin"`
	Resolved        []string `json:"resolved"`
	ResolvedRecipes []string `json:"resolvedRecipes"`
	// Changes is how many things differ on this PC since the profile was last applied; 0 when none or never recorded.
	Changes int `json:"changes"`
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
	a.rememberBaseline(installed)

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
	st.Pending = a.livePending(installed)
	st.StartupError = a.StartupError()
	store := a.profileStore()
	if store == nil {
		a.attachDrift(&st, installed)
		return st, nil
	}
	user, err := store.List(a.cat)
	if err != nil {
		a.attachDrift(&st, installed)
		return st, err
	}
	for _, p := range user {
		st.Profiles = append(st.Profiles, a.profileInfo(p, false))
	}
	a.attachDrift(&st, installed)
	return st, nil
}

// attachDrift fills in how many things changed since each profile was applied.
func (a *App) attachDrift(st *State, installed map[string]system.Installed) {
	apps := make(map[string][]string, len(st.Profiles))
	for _, p := range st.Profiles {
		apps[p.ID] = p.Resolved
	}
	for i, n := range a.driftCounts(apps, installed) {
		for j := range st.Profiles {
			if st.Profiles[j].ID == i {
				st.Profiles[j].Changes = n
			}
		}
	}
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
	return a.start(plan, p.Name, a.wholeProfileID(p))
}

// start runs a plan in the background; only one runs at a time.
func (a *App) start(plan install.Plan, title, baselineFor string) error {
	return a.startRun(plan, title, baselineFor, nil)
}

// startRun is start for plans that may hold offline kit steps; kit answers them. A plan with kit steps is
// not remembered as an unfinished run: its files may be on a stick that is gone.
func (a *App) startRun(plan install.Plan, title, baselineFor string, kit install.KitResolver) error {
	remember := !install.HasKitSteps(plan)
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return errors.New("an installation is already running")
	}
	a.running = true
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.mu.Unlock()

	// The whole plan is stored first and shrinks as steps finish, so whatever
	// is left when the run ends - or when the PC restarts mid-run - can be resumed.
	if remember {
		a.savePending(install.Pending{Title: title, Steps: plan.Steps})
	}
	done := map[string]bool{}
	hist := a.historyBegin(plan, title) // read before anything runs, so "before" is true

	go func() {
		failed := install.RunWith(ctx, a.cat, plan, install.OSExecutor{}, kit, func(e install.Event) {
			hist.Observe(e)
			if e.Status == install.StatusOK && remember {
				done[install.StepKey(e.Step)] = true
				a.savePending(install.Pending{Title: title, Steps: install.Without(plan.Steps, done)})
			}
			runtime.EventsEmit(a.ctx, "install", e)
		})
		if c, ok := kit.(interface{ Close() }); ok {
			c.Close() // the private copies of the kit's installers
		}
		a.historyFinish(hist)
		a.mu.Lock()
		a.running = false
		// A whole profile that finished without a failure and was not stopped: the scan the interface asks for
		// next is the state the profile left the PC in, and becomes its baseline.
		if baselineFor != "" && len(failed) == 0 && ctx.Err() == nil {
			a.baselineFor = baselineFor
		}
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
	store := a.profileStore()
	if store == nil {
		return errStorageUnavailable
	}
	return store.Save(p)
}

// DeleteProfile removes a user profile.
func (a *App) DeleteProfile(id string) error {
	store := a.profileStore()
	if store == nil {
		return errStorageUnavailable
	}
	if err := store.Delete(id); err != nil {
		return err
	}
	if a.applied != nil {
		if err := a.applied.Remove(id); err != nil {
			log.Printf("winforge: forgetting the history of %s: %v", id, err)
		}
	}
	return nil
}

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

// ExportConfigure writes the profile's apps as a "winget configure" file. Recipes are left out: they
// run scripts, which do not belong in a file another person will apply.
func (a *App) ExportConfigure(p catalog.Profile) (string, error) {
	res, err := a.cat.ResolveProfile(&p)
	if err != nil {
		return "", err
	}
	return a.saveWith(p.ID+".winget", "winget configuration (*.winget)", "*.winget", func() ([]byte, error) {
		return profile.WingetConfigure(p.Name, res.Apps, res.Recipes, a.cat)
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

// ApplyUpgrades updates the given catalog apps in the background.
func (a *App) ApplyUpgrades(ids []string) error {
	plan, err := a.PlanUpgrades(ids)
	if err != nil {
		return err
	}
	if len(plan.Steps) == 0 {
		return errors.New("nothing to update")
	}
	return a.start(plan, "updates", "")
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
