package main

import (
	"errors"
	"sync"

	"github.com/Sebas1705/WinForge/internal/privacy"
)

// privacySession remembers that the Privacy page listed its settings, so a change can only name
// ids that were shown to the person. The ids are the fixed curated list: the interface can never
// name a registry value of its own.
type privacySession struct {
	mu     sync.Mutex
	listed map[string]bool
	op     sync.Mutex // one change at a time
}

var (
	errPrivacyNotListed = errors.New("that setting was not in the last list: refresh the page and try again")
	errPrivacyNothing   = errors.New("no setting was chosen")
	errPrivacyNoBackup  = errors.New("that backup is not in the list: refresh the page and try again")
)

func (a *App) privacyManager() *privacy.Manager {
	sys := a.privacySys
	if sys == nil {
		sys = privacy.NewSystem()
	}
	return &privacy.Manager{Sys: sys, Now: a.clock}
}

// PrivacyList reads the current state of every privacy setting from the registry (a value that is
// absent means Windows' default). It only reads, and remembers the ids it returned.
func (a *App) PrivacyList() ([]privacy.TweakView, error) {
	list, err := a.privacyManager().List()
	if err != nil {
		return nil, err
	}
	listed := make(map[string]bool, len(list))
	for _, v := range list {
		listed[v.ID] = true
	}
	a.privacyState.mu.Lock()
	a.privacyState.listed = listed
	a.privacyState.mu.Unlock()
	return list, nil
}

func (a *App) privacyCheck(ids []string) error {
	if len(ids) == 0 {
		return errPrivacyNothing
	}
	a.privacyState.mu.Lock()
	defer a.privacyState.mu.Unlock()
	for _, id := range ids {
		if !a.privacyState.listed[id] {
			return errPrivacyNotListed
		}
	}
	return nil
}

// PrivacyApply switches the chosen settings to their private value. Every id must come from the
// last PrivacyList: one unknown id refuses the whole request and nothing is written. A backup of
// every value that changes is saved first; PrivacyRestore puts it back. Per-user values only.
func (a *App) PrivacyApply(ids []string) (privacy.Result, error) {
	if err := a.privacyCheck(ids); err != nil {
		return privacy.Result{Skipped: []string{}, Failed: []string{}}, err
	}
	a.privacyState.op.Lock()
	defer a.privacyState.op.Unlock()
	return a.privacyManager().Apply(ids)
}

// PrivacyReset gives the chosen settings back to Windows (their values are removed), with a backup
// like PrivacyApply, so it can be undone the same way.
func (a *App) PrivacyReset(ids []string) (privacy.Result, error) {
	if err := a.privacyCheck(ids); err != nil {
		return privacy.Result{Skipped: []string{}, Failed: []string{}}, err
	}
	a.privacyState.op.Lock()
	defer a.privacyState.op.Unlock()
	return a.privacyManager().Reset(ids)
}

// PrivacyBackups lists the saved backups, newest first.
func (a *App) PrivacyBackups() ([]privacy.Backup, error) {
	return a.privacyManager().Backups()
}

// PrivacyRestore puts back exactly what one backup that PrivacyBackups lists holds. It takes an id,
// never a path. Values that were edited since the change are restored too and reported.
func (a *App) PrivacyRestore(id string) (privacy.RestoreResult, error) {
	a.privacyState.op.Lock()
	defer a.privacyState.op.Unlock()
	m := a.privacyManager()
	list, err := m.Backups()
	if err != nil {
		return privacy.RestoreResult{}, err
	}
	for _, b := range list {
		if b.ID == id {
			return m.Restore(id)
		}
	}
	return privacy.RestoreResult{Restored: []string{}, ChangedSince: []string{}, Failed: []string{}}, errPrivacyNoBackup
}
