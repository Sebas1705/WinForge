package main

import (
	"errors"
	"os/exec"

	"github.com/Sebas1705/WinForge/internal/install"
)

// PlanRepairs previews fixing findings by themselves: which steps would run and
// whether they need administrator rights. Only repair recipes are accepted.
func (a *App) PlanRepairs(ids []string) (install.Plan, error) {
	return install.RepairPlan(a.cat, ids)
}

// ApplyRepairs runs the repairs in the background, with the same progress,
// stop and retry as an installation.
func (a *App) ApplyRepairs(ids []string) error {
	plan, err := install.RepairPlan(a.cat, ids)
	if err != nil {
		return err
	}
	if len(plan.Steps) == 0 {
		return errors.New("nothing to repair")
	}
	return a.start(plan, "repair")
}

// RestartPC restarts Windows in 30 seconds, which leaves time to cancel with
// "shutdown /a". The interface asks before calling it.
func (a *App) RestartPC() error {
	return exec.Command("shutdown.exe", "/r", "/t", "30", "/c", "WinForge: restarting to finish repairs").Run()
}

// CancelRestart cancels a restart started by RestartPC.
func (a *App) CancelRestart() error {
	return exec.Command("shutdown.exe", "/a").Run()
}
