package main

import (
	"errors"
	"sync"

	"github.com/Sebas1705/WinForge/internal/envdoctor"
)

// envSession remembers which findings the last EnvScan offered a fix for, so the interface can only
// ask for fixes that were shown to the person: it cannot name a variable or a PATH entry of its own.
type envSession struct {
	mu      sync.Mutex
	fixable map[string]bool
	op      sync.Mutex // one change at a time
}

var (
	errEnvNotListed   = errors.New("that fix was not in the last scan: scan again and try once more")
	errEnvNothingPick = errors.New("no fix was chosen")
	errEnvNoBackup    = errors.New("that backup is not in the list: refresh the page and try again")
)

func (a *App) envManager() *envdoctor.Manager {
	sys := a.envSys
	if sys == nil {
		sys = envdoctor.NewSystem()
	}
	return &envdoctor.Manager{Sys: sys, Now: a.clock}
}

// EnvScan reads the user and machine environment and reports what looks wrong. It only reads, and
// never runs a program found on PATH. It remembers the ids of the findings that can be fixed:
// only those can be passed to EnvFix.
func (a *App) EnvScan() (envdoctor.Report, error) {
	rep, err := a.envManager().Scan()
	if err != nil {
		return envdoctor.Report{}, err
	}
	fixable := map[string]bool{}
	for _, f := range rep.Findings {
		if f.Fixable {
			fixable[f.ID] = true
		}
	}
	a.envState.mu.Lock()
	a.envState.fixable = fixable
	a.envState.mu.Unlock()
	return rep, nil
}

// EnvFix applies fixes from the last scan to the person's own variables. Every id must come from
// that scan: one unknown id refuses the whole request and nothing is written. A backup of what is
// replaced is saved first; EnvRestore puts it back.
func (a *App) EnvFix(ids []string) (envdoctor.FixResult, error) {
	if len(ids) == 0 {
		return envdoctor.FixResult{Failed: []string{}}, errEnvNothingPick
	}
	a.envState.mu.Lock()
	for _, id := range ids {
		if !a.envState.fixable[id] {
			a.envState.mu.Unlock()
			return envdoctor.FixResult{Failed: []string{}}, errEnvNotListed
		}
	}
	a.envState.mu.Unlock()

	a.envState.op.Lock()
	defer a.envState.op.Unlock()
	res, err := a.envManager().Fix(ids)
	// The scan the ids came from is out of date now, whatever happened.
	a.envState.mu.Lock()
	a.envState.fixable = nil
	a.envState.mu.Unlock()
	return res, err
}

// EnvBackups lists the saved backups, newest first.
func (a *App) EnvBackups() ([]envdoctor.Backup, error) {
	return a.envManager().Backups()
}

// EnvRestore writes back the values of one backup that EnvBackups lists. It takes an id, never a
// path. Variables that were changed again since the fix are restored too.
func (a *App) EnvRestore(id string) error {
	a.envState.op.Lock()
	defer a.envState.op.Unlock()
	m := a.envManager()
	list, err := m.Backups()
	if err != nil {
		return err
	}
	listed := false
	for _, b := range list {
		if b.ID == id {
			listed = true
			break
		}
	}
	if !listed {
		return errEnvNoBackup
	}
	_, err = m.Restore(id)
	a.envState.mu.Lock()
	a.envState.fixable = nil
	a.envState.mu.Unlock()
	return err
}
