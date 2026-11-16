package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/system"
)

// startupSession remembers what the last StartupList showed, so the interface can only switch or
// reveal items that were listed to the person: it cannot name a registry value or a file of its own.
type startupSession struct {
	mu     sync.Mutex
	listed map[string]startup.Item
	op     sync.Mutex // one change at a time
}

var errStartupNotListed = errors.New("that startup item was not in the last list: refresh the page and try again")

func (a *App) startupManager() *startup.Manager {
	sys := a.startupSys
	if sys == nil {
		sys = startup.NewSystem()
	}
	elevated := a.elevated
	if elevated == nil {
		elevated = system.IsElevated
	}
	return &startup.Manager{Sys: sys, Elevated: elevated, Now: a.clock}
}

// StartupList lists what starts with Windows and remembers the ids it returned: only those can be
// switched afterwards.
func (a *App) StartupList() ([]startup.Item, error) {
	items, err := a.startupManager().List()
	if err != nil {
		return nil, err
	}
	listed := make(map[string]startup.Item, len(items))
	for _, it := range items {
		listed[it.ID] = it
	}
	a.startupState.mu.Lock()
	a.startupState.listed = listed
	a.startupState.mu.Unlock()
	return items, nil
}

// StartupSet switches a listed item on or off the way Task Manager does and returns the refreshed
// list. The item is read again before anything is written, machine items are refused with
// "needs administrator" unless the app is elevated, and asking for the state an item already has
// changes nothing. Nothing is deleted: the Run value or file stays.
func (a *App) StartupSet(id string, enabled bool) ([]startup.Item, error) {
	a.startupState.mu.Lock()
	_, ok := a.startupState.listed[id]
	a.startupState.mu.Unlock()
	if !ok {
		return nil, errStartupNotListed
	}
	a.startupState.op.Lock()
	defer a.startupState.op.Unlock()
	if err := a.startupManager().Toggle(id, enabled); err != nil {
		if errors.Is(err, startup.ErrNotFound) {
			a.startupState.mu.Lock()
			delete(a.startupState.listed, id)
			a.startupState.mu.Unlock()
		}
		return nil, err
	}
	return a.StartupList()
}

// StartupReveal shows the program (or the file in the Startup folder) of a listed item in Explorer.
func (a *App) StartupReveal(id string) error {
	a.startupState.mu.Lock()
	it, ok := a.startupState.listed[id]
	a.startupState.mu.Unlock()
	if !ok {
		return errStartupNotListed
	}
	if it.Target == "" {
		return errors.New("this item does not start a file that can be shown")
	}
	sys := a.startupSys
	if sys == nil {
		sys = startup.NewSystem()
	}
	if !sys.Exists(it.Target) {
		return errors.New("the file is not there any more")
	}
	if a.reveal != nil {
		return a.reveal(it.Target)
	}
	// explorer.exe reports failure even when it opens, so its exit code is ignored.
	return exec.Command("explorer.exe", "/select,"+filepath.Clean(it.Target)).Start()
}
