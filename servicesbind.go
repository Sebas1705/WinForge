package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/internal/origstate"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/tasks"
)

// svcFeature holds the Services and Scheduled tasks tabs of the Startup page: what each last list
// showed (so the interface can only change items that were listed to the person), and the
// machine behind them. The system fields, elevated and stateDir are replaced by tests (nil means
// the real service manager, task scheduler, token and %AppData%).
type svcFeature struct {
	services servicesSession
	tasks    tasksSession

	servicesSys services.System
	tasksSys    tasks.System
	// stateDir returns the folder for the "original state" files; nil means %AppData%\WinForge.
	stateDir func() (string, error)
}

// servicesSession remembers what the last ServicesList showed.
type servicesSession struct {
	mu     sync.Mutex
	listed map[string]bool
	op     sync.Mutex // one change at a time
}

var errServiceNotListed = errors.New("svc:not-listed")

func (a *App) origStore(name string) *origstate.Store {
	if a.svc.stateDir != nil {
		dir, err := a.svc.stateDir()
		if err != nil {
			return nil
		}
		return &origstate.Store{Path: filepath.Join(dir, name)}
	}
	p, err := origstate.DefaultPath(name)
	if err != nil {
		return nil
	}
	return &origstate.Store{Path: p}
}

func (a *App) servicesManager() *services.Manager {
	sys := a.svc.servicesSys
	if sys == nil {
		sys = services.NewSystem()
	}
	elevated := a.elevated
	if elevated == nil {
		elevated = system.IsElevated
	}
	return &services.Manager{Sys: sys, Elevated: elevated, Store: a.origStore("services-original.json")}
}

// ServicesList lists the third-party services and remembers the ids it returned: only those can be
// changed afterwards.
func (a *App) ServicesList() ([]services.Item, error) {
	items, err := a.servicesManager().List()
	if err != nil {
		return nil, err
	}
	listed := make(map[string]bool, len(items))
	for _, it := range items {
		listed[strings.ToLower(it.ID)] = true
	}
	a.svc.services.mu.Lock()
	a.svc.services.listed = listed
	a.svc.services.mu.Unlock()
	return items, nil
}

func (a *App) servicesIsListed(id string) bool {
	a.svc.services.mu.Lock()
	defer a.svc.services.mu.Unlock()
	return a.svc.services.listed[strings.ToLower(id)]
}

// ServicesSetMode changes the start mode of a listed service ("auto", "delayed", "manual" or
// "disabled") and returns the refreshed list. The service is read and classified again before
// anything is written, so a hidden or protected one is refused even if its name was guessed.
// Administrator rights are required; a security product, or a service other services depend on
// when its start mode is lowered, is refused unless confirmed. Nothing is stopped or deleted.
func (a *App) ServicesSetMode(id, mode string, confirmed bool) ([]services.Item, error) {
	if !a.servicesIsListed(id) {
		return nil, errServiceNotListed
	}
	a.svc.services.op.Lock()
	defer a.svc.services.op.Unlock()
	if err := a.servicesManager().SetMode(id, services.Mode(mode), confirmed); err != nil {
		return nil, a.serviceFailed(id, err)
	}
	return a.ServicesList()
}

// ServicesRestore puts a listed service back to the start mode it had before WinForge changed it.
func (a *App) ServicesRestore(id string, confirmed bool) ([]services.Item, error) {
	if !a.servicesIsListed(id) {
		return nil, errServiceNotListed
	}
	a.svc.services.op.Lock()
	defer a.svc.services.op.Unlock()
	if err := a.servicesManager().Restore(id, confirmed); err != nil {
		return nil, a.serviceFailed(id, err)
	}
	return a.ServicesList()
}

// serviceFailed forgets an id that is gone or protected so it cannot be retried.
func (a *App) serviceFailed(id string, err error) error {
	if errors.Is(err, services.ErrNotFound) || errors.Is(err, services.ErrProtected) {
		a.svc.services.mu.Lock()
		delete(a.svc.services.listed, strings.ToLower(id))
		a.svc.services.mu.Unlock()
	}
	return err
}
