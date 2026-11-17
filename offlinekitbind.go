package main

import (
	"context"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/offlinekit"
)

// kitState is what the offline kit feature remembers between calls. Installing only accepts ids that the last
// KitOpen verified, and runs only what that kit's kit.json names.
type kitState struct {
	mu     sync.Mutex
	busy   bool // a create or an open is running: one long kit task at a time
	cancel context.CancelFunc
	folder string
	opened *offlinekit.Report // the kit KitOpen verified, nil when none is open
	sess   *offlinekit.Session

	// Replaced by tests: nil means the real disk, the real `winget download` and the real runner.
	fs    offlinekit.FS
	dl    offlinekit.Downloader
	start func(plan install.Plan, title string, kit install.KitResolver) error
}

func (a *App) kitFS() offlinekit.FS {
	if a.kit.fs != nil {
		return a.kit.fs
	}
	return offlinekit.OSFS{}
}

func (a *App) kitDownloader() offlinekit.Downloader {
	if a.kit.dl != nil {
		return a.kit.dl
	}
	return offlinekit.WingetDownloader{Ex: install.OSExecutor{}}
}

// beginKitTask starts a long kit task: only one runs at a time and KitCancel stops it.
func (a *App) beginKitTask() (context.Context, func(), error) {
	a.kit.mu.Lock()
	defer a.kit.mu.Unlock()
	if a.kit.busy {
		return nil, nil, &offlinekit.Error{Code: offlinekit.CodeBusy}
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.kit.busy, a.kit.cancel = true, cancel
	return ctx, func() {
		cancel()
		a.kit.mu.Lock()
		a.kit.busy, a.kit.cancel = false, nil
		a.kit.mu.Unlock()
	}, nil
}

// KitCancel stops the kit task that is running (creating or checking a kit). Stopping an install is Cancel.
func (a *App) KitCancel() {
	a.kit.mu.Lock()
	defer a.kit.mu.Unlock()
	if a.kit.cancel != nil {
		a.kit.cancel()
	}
}

func (a *App) kitProgress(p offlinekit.Progress) { a.emitEvent("kit:progress", p) }

// known says whether a kit's app is an app of this catalog with that winget id: a kit can only name apps
// WinForge knows, so a hand-edited kit.json cannot add software the catalog does not list.
func (a *App) kitKnown(id, winget string) bool {
	app := a.cat.Apps[id]
	return app != nil && strings.EqualFold(app.Winget, winget)
}

// KitPrepare says what creating a kit of these apps in folder would do and whether the drive has room, and
// changes nothing. Store apps and ids the catalog does not have come back as not included.
func (a *App) KitPrepare(ids []string, folder string) offlinekit.CreatePlan {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return offlinekit.Plan(ctx, a.kitFS(), a.cat, ids, folder)
}

// KitCreateResult is what a creation did.
type KitCreateResult struct {
	Added       []string                 `json:"added"`
	Failed      []offlinekit.NotIncluded `json:"failed"`
	TotalApps   int                      `json:"totalApps"`
	Bytes       int64                    `json:"bytes"`
	Cancelled   bool                     `json:"cancelled"`
	Fingerprint string                   `json:"fingerprint"`
	Folder      string                   `json:"folder"`
}

// KitCreate downloads the apps' installers into folder with `winget download` and writes kit.json. It runs
// as the one long kit task: "kit:progress" events report the app being fetched and KitCancel stops it, keeping
// what finished. A failed app is reported, not fatal. Ids must be catalog apps; nothing else is downloaded.
func (a *App) KitCreate(ids []string, folder, profileName string) (KitCreateResult, error) {
	ctx, end, err := a.beginKitTask()
	if err != nil {
		return KitCreateResult{}, err
	}
	defer end()
	res, err := offlinekit.Create(ctx, offlinekit.Options{
		FS: a.kitFS(), Dl: a.kitDownloader(), Cat: a.cat, Version: version, Now: a.clock, Progress: a.kitProgress,
	}, ids, folder, strings.TrimSpace(profileName))
	if err != nil {
		return KitCreateResult{}, err
	}
	out := KitCreateResult{Added: res.Added, Failed: res.Failed, Bytes: res.Bytes, Cancelled: res.Cancelled, Fingerprint: res.Fingerprint, Folder: folder}
	if out.Added == nil {
		out.Added = []string{}
	}
	if out.Failed == nil {
		out.Failed = []offlinekit.NotIncluded{}
	}
	if res.Kit != nil {
		out.TotalApps = len(res.Kit.Apps)
	}
	return out, nil
}

// KitApp is an app of an opened kit as the page lists it.
type KitApp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Type      string `json:"type"`
	SizeBytes int64  `json:"sizeBytes"`
	Admin     bool   `json:"admin,omitempty"`
	// State is ready, problem or unsupported (see offlinekit).
	State    string               `json:"state"`
	Problems []offlinekit.Problem `json:"problems"`
	// Reason says why an intact app cannot be installed offline (type or switches), Detail names the installer type.
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Installed is true when the app is already on this PC; it is then not offered.
	Installed        bool   `json:"installed,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

// KitListing is an opened kit: what it holds, what is wrong with it and what could not be included.
type KitListing struct {
	Folder      string                   `json:"folder"`
	Fingerprint string                   `json:"fingerprint"`
	Created     string                   `json:"created"`
	Updated     string                   `json:"updated,omitempty"`
	WinForge    string                   `json:"winforge"`
	Profile     string                   `json:"profile,omitempty"`
	Apps        []KitApp                 `json:"apps"`
	NotIncluded []offlinekit.NotIncluded `json:"notIncluded"`
	// Unlisted are names in the folder that kit.json does not mention; they are ignored.
	Unlisted []string `json:"unlisted"`
	// NeedsAdmin is true when an app that could be installed needs administrator rights.
	NeedsAdmin bool `json:"needsAdmin"`
}

// KitOpen reads the kit in folder and verifies every file (SHA-256 against kit.json and against the winget
// manifest, no links, nothing unlisted). Nothing is run. It returns the listing, with what is wrong with each
// app; the apps that verified are the only ones KitInstall accepts. It is the one long kit task: progress is
// reported and KitCancel stops it.
func (a *App) KitOpen(folder string) (KitListing, error) {
	ctx, end, err := a.beginKitTask()
	if err != nil {
		return KitListing{}, err
	}
	defer end()
	a.kitForget()
	rep, err := offlinekit.Verify(ctx, a.kitFS(), folder, a.kitKnown, a.kitProgress)
	if err != nil {
		return KitListing{}, err
	}
	installed, err := a.freshInstalled()
	if err != nil {
		return KitListing{}, err
	}
	a.kit.mu.Lock()
	a.kit.folder, a.kit.opened = folder, rep
	a.kit.mu.Unlock()
	k := rep.Kit
	out := KitListing{
		Folder: folder, Fingerprint: rep.Fingerprint, Created: k.Created, Updated: k.Updated, WinForge: k.WinForge, Profile: k.Profile,
		Apps: []KitApp{}, NotIncluded: k.NotIncluded, Unlisted: rep.Unlisted,
	}
	for _, ar := range rep.Apps {
		e := ar.Entry
		ka := KitApp{
			ID: e.ID, Name: e.Name, Version: e.Version, Type: e.Installer.Type, SizeBytes: e.Size, Admin: e.Admin,
			State: ar.State, Problems: ar.Problems, Reason: ar.Unsupported, Detail: ar.UnsupportedDetail,
		}
		if ka.Problems == nil {
			ka.Problems = []offlinekit.Problem{}
		}
		if app := a.cat.Apps[e.ID]; app != nil {
			ka.Name = app.Name
		}
		if in, ok := installed[e.ID]; ok {
			ka.Installed, ka.InstalledVersion = true, in.Version
		}
		if ar.State == offlinekit.StateReady && !ka.Installed && e.Admin {
			out.NeedsAdmin = true
		}
		out.Apps = append(out.Apps, ka)
	}
	if out.NotIncluded == nil {
		out.NotIncluded = []offlinekit.NotIncluded{}
	}
	if out.Unlisted == nil {
		out.Unlisted = []string{}
	}
	return out, nil
}

// kitForget drops the opened kit and its private copies.
func (a *App) kitForget() {
	a.kit.mu.Lock()
	sess := a.kit.sess
	a.kit.opened, a.kit.sess, a.kit.folder = nil, nil, ""
	a.kit.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}

// kitSelect turns the ticked ids into a plan, from the kit KitOpen verified. An id the kit did not offer is
// refused and nothing is planned.
func (a *App) kitSelect(ids []string) (offlinekit.Selection, string, error) {
	a.kit.mu.Lock()
	rep, folder := a.kit.opened, a.kit.folder
	a.kit.mu.Unlock()
	if rep == nil {
		return offlinekit.Selection{}, "", &offlinekit.Error{Code: offlinekit.CodeNotOpen}
	}
	installed, err := a.freshInstalled()
	if err != nil {
		return offlinekit.Selection{}, "", err
	}
	sel, err := offlinekit.BuildPlan(a.cat, rep, uniqueIDs(ids), installed)
	return sel, folder, err
}

func uniqueIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// KitPlan is what installing the ticked apps would do on this PC: one kit step per app, in the kit's order,
// and what is already installed.
func (a *App) KitPlan(ids []string) (install.Plan, error) {
	sel, _, err := a.kitSelect(ids)
	return sel.Plan, err
}

// KitInstall installs the ticked apps from the opened kit through the same runner as every install ("install"
// events, Cancel, "install:done"). Only ids KitOpen verified are accepted; each installer is copied to a
// private folder, checked against kit.json once more and run with the manifest's silent switches. When the run
// is over the page calls KitVerify.
func (a *App) KitInstall(ids []string) error {
	sel, folder, err := a.kitSelect(ids)
	if err != nil {
		return err
	}
	if len(sel.Plan.Steps) == 0 {
		return &offlinekit.Error{Code: offlinekit.CodeNothing}
	}
	sess := offlinekit.NewSession(a.kitFS(), folder, sel.Apps)
	start := func(plan install.Plan, title string, kit install.KitResolver) error {
		return a.startRun(plan, title, "", kit)
	}
	if a.kit.start != nil {
		start = a.kit.start
	}
	if err := start(sel.Plan, "kit", sess); err != nil {
		sess.Close()
		return err
	}
	a.kit.mu.Lock()
	a.kit.sess = sess
	a.kit.mu.Unlock()
	return nil
}

// KitRef names an app in a verification.
type KitRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// KitVerified is what a re-scan found after an install from a kit: the installer saying it finished is not
// enough, so the PC is looked at again.
type KitVerified struct {
	Installed []KitRef `json:"installed"`
	// Missing are apps the scan still does not detect: the install did not take, or needs a restart first.
	Missing []KitRef `json:"missing"`
}

// KitVerify scans the PC again and says which of the apps of the opened kit are now detected. Ids the opened
// kit did not offer are ignored.
func (a *App) KitVerify(ids []string) (KitVerified, error) {
	a.kit.mu.Lock()
	rep := a.kit.opened
	a.kit.mu.Unlock()
	if rep == nil {
		return KitVerified{}, &offlinekit.Error{Code: offlinekit.CodeNotOpen}
	}
	offered := map[string]bool{}
	for _, ar := range rep.Apps {
		offered[ar.Entry.ID] = true
	}
	installed, err := a.freshInstalled()
	if err != nil {
		return KitVerified{}, err
	}
	out := KitVerified{Installed: []KitRef{}, Missing: []KitRef{}}
	for _, id := range uniqueIDs(ids) {
		app := a.cat.Apps[id]
		if app == nil || !offered[id] {
			continue
		}
		if _, ok := installed[id]; ok {
			out.Installed = append(out.Installed, KitRef{ID: id, Name: app.Name})
		} else {
			out.Missing = append(out.Missing, KitRef{ID: id, Name: app.Name})
		}
	}
	return out, nil
}
