package main

import (
	"errors"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/tasks"
)

// tasksSession remembers what the last TasksList showed.
type tasksSession struct {
	mu     sync.Mutex
	listed map[string]bool // by lower-case id
	op     sync.Mutex      // one change at a time
}

var errTaskNotListed = errors.New("task:not-listed")

func (a *App) tasksManager() *tasks.Manager {
	sys := a.svc.tasksSys
	if sys == nil {
		sys = tasks.NewSystem()
	}
	elevated := a.elevated
	if elevated == nil {
		elevated = system.IsElevated
	}
	return &tasks.Manager{Sys: sys, Elevated: elevated, Store: a.origStore("tasks-original.json")}
}

// TasksList lists the third-party scheduled tasks that start at log on or at startup, and
// remembers the ids it returned: only those can be changed afterwards.
func (a *App) TasksList() ([]tasks.Item, error) {
	items, err := a.tasksManager().List()
	if err != nil {
		return nil, err
	}
	listed := make(map[string]bool, len(items))
	for _, it := range items {
		listed[strings.ToLower(it.ID)] = true
	}
	a.svc.tasks.mu.Lock()
	a.svc.tasks.listed = listed
	a.svc.tasks.mu.Unlock()
	return items, nil
}

func (a *App) taskIsListed(id string) bool {
	a.svc.tasks.mu.Lock()
	defer a.svc.tasks.mu.Unlock()
	return a.svc.tasks.listed[strings.ToLower(id)]
}

// TasksSetEnabled enables or disables a listed task and returns the refreshed list. The task is
// read and classified again before anything is written, so a hidden one (anything under
// \Microsoft\, WinForge's own) is refused even if its id was guessed. Tasks other than the
// user's own limited ones need administrator rights; disabling a security product's task is
// refused unless confirmed. The task is never deleted, run or edited.
func (a *App) TasksSetEnabled(id string, enabled, confirmed bool) ([]tasks.Item, error) {
	if !a.taskIsListed(id) {
		return nil, errTaskNotListed
	}
	a.svc.tasks.op.Lock()
	defer a.svc.tasks.op.Unlock()
	if err := a.tasksManager().SetEnabled(id, enabled, confirmed); err != nil {
		return nil, a.taskFailed(id, err)
	}
	return a.TasksList()
}

// TasksRestore puts a listed task back to the state it had before WinForge changed it.
func (a *App) TasksRestore(id string, confirmed bool) ([]tasks.Item, error) {
	if !a.taskIsListed(id) {
		return nil, errTaskNotListed
	}
	a.svc.tasks.op.Lock()
	defer a.svc.tasks.op.Unlock()
	if err := a.tasksManager().Restore(id, confirmed); err != nil {
		return nil, a.taskFailed(id, err)
	}
	return a.TasksList()
}

func (a *App) taskFailed(id string, err error) error {
	if errors.Is(err, tasks.ErrNotFound) || errors.Is(err, tasks.ErrProtected) {
		a.svc.tasks.mu.Lock()
		delete(a.svc.tasks.listed, strings.ToLower(id))
		a.svc.tasks.mu.Unlock()
	}
	return err
}
