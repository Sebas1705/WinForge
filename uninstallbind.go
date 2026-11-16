package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/uninstall"
)

// uninstallState is what the Uninstall page remembers between calls. Only catalog apps are ever named:
// every id is looked up in the catalog, and the leftovers a search listed are the only paths a removal accepts.
type uninstallState struct {
	mu        sync.Mutex
	op        sync.Mutex      // one search or removal at a time
	requested map[string]bool // apps an uninstall was started for in this session
	removed   []string        // of those, the ones a re-scan confirmed gone, in the order they were
	listed    map[string]uninstall.Candidate
	listedApp string

	// Replaced by tests: nil means this PC's registry and disk, a real re-scan and the real Recycle Bin.
	sys    uninstall.System
	env    *uninstall.Env
	detect func() (map[string]system.Installed, error)
	start  func(plan install.Plan, title, baselineFor string) error
}

var (
	errNotInstalled   = errors.New("that app is not installed, so it cannot be uninstalled")
	errStillInstalled = errors.New("that app is still installed: uninstall it first, then look for what it left")
	errNotListed      = errors.New("that path was not listed by the last search, so it is not moved: search again")
	errNoLeftovers    = errors.New("nothing selected to move")
	errNotCatalog     = errors.New("that is not an app in the catalog")
)

func (a *App) uninstallSystem() uninstall.System {
	if a.uninst.sys != nil {
		return a.uninst.sys
	}
	var run storage.Runner = storage.OSRunner{}
	if a.runner != nil {
		run = a.runner
	}
	return uninstall.NewSystem(run)
}

func (a *App) uninstallEnv() uninstall.Env {
	if a.uninst.env != nil {
		return *a.uninst.env
	}
	return uninstall.DefaultEnv()
}

// freshInstalled looks at the PC again, so what the page offers is what is installed now and not what a
// scan earlier showed. winget being unavailable is not an error here: detection still has the registry.
func (a *App) freshInstalled() (map[string]system.Installed, error) {
	var installed map[string]system.Installed
	if a.uninst.detect != nil {
		var err error
		if installed, err = a.uninst.detect(); err != nil {
			return nil, err
		}
	} else {
		inv, err := system.SnapshotWith(a.ctx, nil)
		if err != nil {
			log.Printf("winforge: uninstall: inventory: %v", err)
		}
		installed = system.Detect(a.cat, inv)
	}
	a.mu.Lock()
	a.installed = installed
	a.mu.Unlock()
	return installed, nil
}

func uninstallApp(app *catalog.App) uninstall.App {
	return uninstall.App{ID: app.ID, Name: app.Name, Publisher: app.Publisher, Winget: app.Winget}
}

// UninstallApp is an installed catalog app as the Uninstall page lists it.
type UninstallApp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Publisher string `json:"publisher"`
	Admin     bool   `json:"admin,omitempty"`
	// SizeBytes is what the installer recorded for the app in Windows' list of installed programs; 0 when it recorded nothing.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
}

// UninstallRemoved is an app WinForge uninstalled in this session that is still gone.
type UninstallRemoved struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UninstallListing is the Uninstall page's content.
type UninstallListing struct {
	Apps []UninstallApp `json:"apps"`
	// Removed lists what WinForge uninstalled in this session and a re-scan confirmed gone: the apps whose leftovers are offered first.
	Removed []UninstallRemoved `json:"removed"`
}

// UninstallList scans the PC again and lists the installed catalog apps, sorted by name, with the size
// Windows records for each when it is known.
func (a *App) UninstallList() (UninstallListing, error) {
	installed, err := a.freshInstalled()
	if err != nil {
		return UninstallListing{}, err
	}
	regs := a.uninstallSystem().Installed()
	out := UninstallListing{Apps: []UninstallApp{}, Removed: []UninstallRemoved{}}
	for _, id := range sortedApps(a.cat) {
		in, ok := installed[id]
		if !ok {
			continue
		}
		app := a.cat.Apps[id]
		out.Apps = append(out.Apps, UninstallApp{
			ID: id, Name: app.Name, Version: in.Version, Publisher: app.Publisher, Admin: app.Admin,
			SizeBytes: uninstall.InstallSize(app.Detect.Registry, regs),
		})
	}
	sort.SliceStable(out.Apps, func(i, j int) bool { return strings.ToLower(out.Apps[i].Name) < strings.ToLower(out.Apps[j].Name) })
	a.uninst.mu.Lock()
	for _, id := range a.uninst.removed {
		if _, back := installed[id]; !back && a.cat.Apps[id] != nil {
			out.Removed = append(out.Removed, UninstallRemoved{ID: id, Name: a.cat.Apps[id].Name})
		}
	}
	a.uninst.mu.Unlock()
	return out, nil
}

// UninstallApps uninstalls catalog apps through the same runner as installs and updates: winget
// uninstall, one step per app, the progress and failures reported as install events. An id that is not an
// installed catalog app is refused and nothing runs. When the run is over the page calls UninstallVerify.
func (a *App) UninstallApps(ids []string) error {
	seen := map[string]bool{}
	var want []string
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if a.cat.Apps[id] == nil {
			return fmt.Errorf("%w: %q", errNotCatalog, id)
		}
		if _, ok := installed[id]; !ok {
			return fmt.Errorf("%w: %q", errNotInstalled, id)
		}
		want = append(want, id)
	}
	if len(want) == 0 {
		return errors.New("nothing to remove")
	}
	plan := install.UninstallPlan(a.cat, want)
	// Remembered before the run starts, so a verification can never arrive before the request is known.
	a.uninst.mu.Lock()
	if a.uninst.requested == nil {
		a.uninst.requested = map[string]bool{}
	}
	before := map[string]bool{}
	for _, id := range want {
		before[id] = a.uninst.requested[id]
		a.uninst.requested[id] = true
	}
	a.uninst.mu.Unlock()
	start := a.start
	if a.uninst.start != nil {
		start = a.uninst.start
	}
	if err := start(plan, "uninstall", ""); err != nil {
		a.uninst.mu.Lock()
		for id, was := range before {
			a.uninst.requested[id] = was
		}
		a.uninst.mu.Unlock()
		return err
	}
	return nil
}

// UninstallVerified is what a re-scan found after an uninstall: winget saying it finished is not enough.
type UninstallVerified struct {
	Removed []UninstallRemoved `json:"removed"`
	// Still are the apps the scan still detects: the uninstall did not take, or the app left something behind that counts as installed.
	Still []UninstallRemoved `json:"still"`
}

// UninstallVerify scans the PC again and says which of the apps an uninstall was started for are really
// gone. Only those become eligible for the automatic leftovers search; ids WinForge never started an
// uninstall for are ignored.
func (a *App) UninstallVerify(ids []string) (UninstallVerified, error) {
	installed, err := a.freshInstalled()
	if err != nil {
		return UninstallVerified{}, err
	}
	out := UninstallVerified{Removed: []UninstallRemoved{}, Still: []UninstallRemoved{}}
	a.uninst.mu.Lock()
	defer a.uninst.mu.Unlock()
	for _, id := range ids {
		app := a.cat.Apps[id]
		if app == nil || !a.uninst.requested[id] {
			continue
		}
		item := UninstallRemoved{ID: id, Name: app.Name}
		if _, still := installed[id]; still {
			out.Still = append(out.Still, item)
			a.uninst.removed = without(a.uninst.removed, id)
			continue
		}
		out.Removed = append(out.Removed, item)
		if !contains(a.uninst.removed, id) {
			a.uninst.removed = append(a.uninst.removed, id)
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func without(list []string, s string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// LeftoversResult is what a search found for one app.
type LeftoversResult struct {
	AppID      string                `json:"appId"`
	AppName    string                `json:"appName"`
	Candidates []uninstall.Candidate `json:"candidates"`
	// Searched are the places that were read, so the page can say where it looked.
	Searched []uninstall.Searched `json:"searched"`
}

// LeftoversFind looks for folders and shortcuts a catalog app left behind. It only accepts an app that is
// not installed (a re-scan decides, not the page), reads a fixed list of places, matches names exactly and
// changes nothing. What it lists is the only thing LeftoversRecycle will accept.
func (a *App) LeftoversFind(appID string) (LeftoversResult, error) {
	app := a.cat.Apps[appID]
	if app == nil {
		return LeftoversResult{}, errNotCatalog
	}
	a.uninst.op.Lock()
	defer a.uninst.op.Unlock()
	installed, err := a.freshInstalled()
	if err != nil {
		return LeftoversResult{}, err
	}
	if _, ok := installed[appID]; ok {
		return LeftoversResult{}, errStillInstalled
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	f := uninstall.Finder{Sys: a.uninstallSystem(), Env: a.uninstallEnv()}
	found, searched, err := f.Find(ctx, uninstallApp(app), a.othersInstalled(installed, appID))
	if err != nil {
		return LeftoversResult{}, err
	}
	listed := make(map[string]uninstall.Candidate, len(found))
	for _, c := range found {
		listed[pathKey(c.Path)] = c
	}
	a.uninst.mu.Lock()
	a.uninst.listed, a.uninst.listedApp = listed, appID
	a.uninst.mu.Unlock()
	return LeftoversResult{AppID: appID, AppName: app.Name, Candidates: found, Searched: searched}, nil
}

// othersInstalled are the catalog apps that are installed, except the one being searched for.
func (a *App) othersInstalled(installed map[string]system.Installed, except string) []uninstall.App {
	var out []uninstall.App
	for _, id := range sortedApps(a.cat) {
		if _, ok := installed[id]; ok && id != except {
			out = append(out, uninstallApp(a.cat.Apps[id]))
		}
	}
	return out
}

// LeftoversRecycleResult is what moving leftovers to the Recycle Bin did.
type LeftoversRecycleResult struct {
	Moved      int                 `json:"moved"`
	FreedBytes int64               `json:"freedBytes"`
	Failed     []uninstall.Failure `json:"failed"`
}

// LeftoversRecycle moves leftovers to the Recycle Bin, nothing else. Every path must be one the last
// LeftoversFind listed; each is proved again at the moment it moves, links are never followed, and an app
// that has been installed again is refused. What cannot be moved is reported per path and stays listed.
func (a *App) LeftoversRecycle(paths []string) (LeftoversRecycleResult, error) {
	if len(paths) == 0 {
		return LeftoversRecycleResult{}, errNoLeftovers
	}
	a.uninst.op.Lock()
	defer a.uninst.op.Unlock()
	a.uninst.mu.Lock()
	appID := a.uninst.listedApp
	var items []uninstall.Candidate
	seen := map[string]bool{}
	for _, p := range paths {
		c, ok := a.uninst.listed[pathKey(p)]
		if !ok {
			a.uninst.mu.Unlock()
			return LeftoversRecycleResult{}, fmt.Errorf("%w: %q", errNotListed, p)
		}
		if !seen[pathKey(p)] {
			seen[pathKey(p)] = true
			items = append(items, c)
		}
	}
	a.uninst.mu.Unlock()
	app := a.cat.Apps[appID]
	if app == nil {
		return LeftoversRecycleResult{}, errNotListed
	}
	installed, err := a.freshInstalled()
	if err != nil {
		return LeftoversRecycleResult{}, err
	}
	if _, ok := installed[appID]; ok {
		return LeftoversRecycleResult{}, errStillInstalled
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	f := uninstall.Finder{Sys: a.uninstallSystem(), Env: a.uninstallEnv()}
	res := f.Remove(ctx, uninstallApp(app), a.othersInstalled(installed, appID), items)
	failed := map[string]bool{}
	for _, x := range res.Failed {
		if x.Reason != uninstall.ReasonMissing { // already gone: nothing left to try again
			failed[pathKey(x.Path)] = true
		}
	}
	a.uninst.mu.Lock()
	for _, c := range items {
		if !failed[pathKey(c.Path)] {
			delete(a.uninst.listed, pathKey(c.Path))
		}
	}
	a.uninst.mu.Unlock()
	return LeftoversRecycleResult{Moved: res.Moved, FreedBytes: res.FreedBytes, Failed: res.Failed}, nil
}
