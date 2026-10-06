package main

import (
	"bytes"
	"errors"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
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
