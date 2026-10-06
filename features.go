package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/settings"
	"github.com/Sebas1705/WinForge/internal/system"
)

// livePending is the stored unfinished run minus what no longer applies: an
// install whose app is now there, or an uninstall whose app is gone. When
// nothing is left the stored run is forgotten.
func (a *App) livePending(installed map[string]system.Installed) *install.Pending {
	p := a.pending.Load()
	if p == nil {
		return nil
	}
	keep := []install.Step{}
	for _, s := range p.Steps {
		_, have := installed[s.ID]
		switch {
		case s.Kind == install.StepApp && have:
		case s.Kind == install.StepUninstall && !have:
		case s.Kind != install.StepRecipe && a.cat.Apps[s.ID] == nil:
		case s.Kind == install.StepRecipe && a.cat.Recipes[s.ID] == nil:
		default:
			keep = append(keep, s)
		}
	}
	if len(keep) == 0 {
		_ = a.pending.Clear()
		return nil
	}
	p.Steps = keep
	return p
}

// ResumePending runs what the last run left undone.
func (a *App) ResumePending() error {
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	p := a.livePending(installed)
	if p == nil {
		return errors.New("nothing to continue")
	}
	plan := install.Plan{Steps: p.Steps, AlreadyInstalled: []string{}}
	for _, s := range p.Steps {
		plan.NeedsAdmin = plan.NeedsAdmin || s.Admin
	}
	return a.start(plan, p.Title)
}

// PlanPending previews what ResumePending would do, so the person sees the steps first.
func (a *App) PlanPending() install.Plan {
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	plan := install.Plan{Steps: []install.Step{}, AlreadyInstalled: []string{}}
	if p := a.livePending(installed); p != nil {
		plan.Steps = p.Steps
		for _, s := range p.Steps {
			plan.NeedsAdmin = plan.NeedsAdmin || s.Admin
		}
	}
	return plan
}

// DiscardPending forgets the unfinished run.
func (a *App) DiscardPending() error { return a.pending.Clear() }

// PlanUninstall previews removing catalog apps.
func (a *App) PlanUninstall(ids []string) install.Plan { return install.UninstallPlan(a.cat, ids) }

// ApplyUninstall removes catalog apps in the background.
func (a *App) ApplyUninstall(ids []string) error {
	plan := install.UninstallPlan(a.cat, ids)
	if len(plan.Steps) == 0 {
		return errors.New("nothing to remove")
	}
	return a.start(plan, "uninstall")
}

// InstallWinget gets winget going on a PC that lacks it: first by registering
// the App Installer package Windows ships, then by opening its Store page.
func (a *App) InstallWinget() error {
	script := `Add-AppxPackage -RegisterByFamilyName -MainPackage Microsoft.DesktopAppInstaller_8wekyb3d8bbwe -ErrorAction Stop`
	cmd := exec.Command("powershell", install.PowerShellArgs(script)...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err == nil {
			if _, e := exec.LookPath("winget"); e == nil {
				return nil
			}
		}
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
	}
	// Registering was not enough: the Store has the real installer.
	runtime.BrowserOpenURL(a.ctx, "https://apps.microsoft.com/detail/9nblggh4nns1")
	return errors.New("winget is not set up yet: the Microsoft Store page for App Installer was opened; install it there, then scan again")
}

// ShareCode packs a profile into one line of text for pasting into a message.
func (a *App) ShareCode(p catalog.Profile) (string, error) {
	if _, err := a.cat.ResolveProfile(&p); err != nil {
		return "", err
	}
	return profile.EncodeCode(p)
}

// ImportCode validates a pasted share code like an imported file. It saves nothing.
func (a *App) ImportCode(code string) (*ImportResult, error) {
	imp, err := profile.DecodeCode(code, a.cat)
	if err != nil {
		return nil, err
	}
	return &ImportResult{Profile: imp.Profile, UnknownApps: imp.UnknownApps, UnknownRecipes: imp.UnknownRecipes}, nil
}

// BackupSets lists the app settings that exist on this PC and can be saved.
func (a *App) BackupSets() []settings.Found {
	found := settings.Detect(settings.DefaultRoots(), settings.OSRunner{})
	if found == nil {
		return []settings.Found{}
	}
	return found
}

// BackupSettings asks where to save and writes the chosen sets as a zip. It
// returns the path, or "" if the dialog was cancelled.
func (a *App) BackupSettings(ids []string) (string, error) {
	var buf bytes.Buffer
	if _, err := settings.Backup(&buf, ids, settings.DefaultRoots(), settings.OSRunner{}); err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "winforge-settings.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "WinForge settings (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, buf.Bytes(), 0o644)
}

// RestorePreview is what a settings backup holds, shown before anything is written.
type RestorePreview struct {
	Sets []settings.Found `json:"sets"`
}

// PickRestore asks for a settings backup and reports what is in it. Nothing is
// written; RestoreSettings does that for the sets the person keeps ticked.
func (a *App) PickRestore() (*RestorePreview, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "WinForge settings (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	m, err := settings.ReadManifest(f, fi.Size())
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.restorePath = path
	a.mu.Unlock()
	return &RestorePreview{Sets: m.Sets}, nil
}

// RestoreSettings writes the chosen sets of the backup picked by PickRestore.
// Files it replaces are kept next to the new ones with a .winforge-bak suffix.
func (a *App) RestoreSettings(ids []string) (*settings.Result, error) {
	a.mu.Lock()
	path := a.restorePath
	a.mu.Unlock()
	if path == "" {
		return nil, errors.New("choose a backup first")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return settings.Restore(f, fi.Size(), ids, settings.DefaultRoots(), settings.OSRunner{})
}
