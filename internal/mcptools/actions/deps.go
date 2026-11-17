package actions

import (
	"context"
	"time"

	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/origstate"
	"github.com/Sebas1705/WinForge/internal/privacy"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/tasks"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// StorageOps is what the storage tools need from the disk. The real one measures and cleans the fixed
// categories of internal/storage; tests give a fake that records every call.
type StorageOps interface {
	// Scan measures every category without changing anything.
	Scan(ctx context.Context, progress storage.ProgressFunc) ([]storage.Category, error)
	// Clean removes the contents of the given category ids (fixed categories only).
	Clean(ctx context.Context, ids []string, progress storage.ProgressFunc) storage.Result
	// FindProjectJunk lists build and dependency folders of projects left alone for minAge. It reads only.
	FindProjectJunk(ctx context.Context, roots []string, minAge time.Duration, progress storage.ProgressFunc) ([]storage.ProjectJunk, error)
	// RemoveProjectJunk deletes the given folders, proving each one again at the moment of removal.
	RemoveProjectJunk(ctx context.Context, paths []string, progress storage.ProgressFunc) storage.ProjectResult
}

// PrivacyOps is the privacy engine: *privacy.Manager.
type PrivacyOps interface {
	List() ([]privacy.TweakView, error)
	Preview(ids []string, action string) ([]privacy.PlannedChange, privacy.Result, error)
	Apply(ids []string) (privacy.Result, error)
	Reset(ids []string) (privacy.Result, error)
	Backups() ([]privacy.Backup, error)
	Restore(id string) (privacy.RestoreResult, error)
}

// EnvOps is the environment doctor: *envdoctor.Manager.
type EnvOps interface {
	Scan() (envdoctor.Report, error)
	Fix(ids []string) (envdoctor.FixResult, error)
	Backups() ([]envdoctor.Backup, error)
	Restore(id string) (envdoctor.RestoreResult, error)
}

// StartupOps is the startup manager: *startup.Manager.
type StartupOps interface {
	List() ([]startup.Item, error)
	Toggle(id string, enabled bool) error
}

// ServicesOps is the services manager: *services.Manager.
type ServicesOps interface {
	List() ([]services.Item, error)
	SetMode(id string, mode services.Mode, confirmed bool) error
	Restore(id string, confirmed bool) error
}

// TasksOps is the scheduled tasks manager: *tasks.Manager.
type TasksOps interface {
	List() ([]tasks.Item, error)
	SetEnabled(id string, enabled, confirmed bool) error
	Restore(id string, confirmed bool) error
}

// Deps is everything this group touches besides mcpkit.Env. A nil field makes the tools that need it answer
// that they are not available; Tools fills all of them with the real machine.
type Deps struct {
	Storage  StorageOps
	Privacy  PrivacyOps
	Env      EnvOps
	Startup  StartupOps
	Services ServicesOps
	Tasks    TasksOps
	// Features reads the state of the Windows optional features (it only reads).
	Features func() []winfeatures.Feature
	// StorageEnv is where this PC keeps what the storage categories work on; it is only used to describe them.
	StorageEnv storage.Env
}

// RealDeps is the real machine: the same engines the GUI uses, without its session state.
func RealDeps(env *mcpkit.Env) Deps {
	elevated := func() bool { return env != nil && env.Elevated != nil && env.Elevated() }
	store := func(name string) *origstate.Store {
		p, err := origstate.DefaultPath(name)
		if err != nil {
			return nil
		}
		return &origstate.Store{Path: p}
	}
	senv := storage.DefaultEnv()
	return Deps{
		Storage:    realStorage{env: senv, admin: elevated, now: time.Now},
		Privacy:    &privacy.Manager{Sys: privacy.NewSystem()},
		Env:        &envdoctor.Manager{Sys: envdoctor.NewSystem()},
		Startup:    &startup.Manager{Sys: startup.NewSystem(), Elevated: elevated},
		Services:   &services.Manager{Sys: services.NewSystem(), Elevated: elevated, Store: store("services-original.json")},
		Tasks:      &tasks.Manager{Sys: tasks.NewSystem(), Elevated: elevated, Store: store("tasks-original.json")},
		Features:   func() []winfeatures.Feature { return winfeatures.List(winfeatures.NewSystem(), elevated()) },
		StorageEnv: senv,
	}
}

// realStorage is internal/storage on this PC.
type realStorage struct {
	env   storage.Env
	admin func() bool
	now   func() time.Time
}

func (r realStorage) options(p storage.ProgressFunc) storage.Options {
	return storage.Options{Env: r.env, Runner: storage.OSRunner{}, Admin: r.admin(), Progress: p}
}

func (r realStorage) Scan(ctx context.Context, p storage.ProgressFunc) ([]storage.Category, error) {
	return storage.Scan(ctx, r.options(p))
}

func (r realStorage) Clean(ctx context.Context, ids []string, p storage.ProgressFunc) storage.Result {
	return storage.Clean(ctx, ids, r.options(p))
}

func (r realStorage) FindProjectJunk(ctx context.Context, roots []string, minAge time.Duration, p storage.ProgressFunc) ([]storage.ProjectJunk, error) {
	return storage.FindProjectJunk(ctx, roots, minAge, r.now(), p)
}

func (r realStorage) RemoveProjectJunk(ctx context.Context, paths []string, p storage.ProgressFunc) storage.ProjectResult {
	return storage.RemoveProjectJunk(ctx, r.env, paths, r.now(), p)
}
