package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/system"
)

// storageSession remembers what the scans showed, so the interface can only act
// on files and folders that were listed to the person: it cannot name any other
// path to move to the Recycle Bin or reveal.
type storageSession struct {
	mu       sync.Mutex
	cancel   context.CancelFunc // of the task that is running, if any
	busy     bool
	listed   map[string]bool // files a scan returned
	folders  map[string]bool // folders a scan returned (reveal only)
	groups   []storage.DupGroup
	projects map[string]bool // build folders the last project search listed: the only ones that can be removed
}

func pathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

// emitEvent sends an event to the interface. Tests replace it.
func (a *App) emitEvent(name string, data any) {
	if a.emitter != nil {
		a.emitter(name, data)
		return
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// storageEnv is where this PC keeps what storage works on. Tests give it a layout in a temporary folder.
func (a *App) storageEnv() storage.Env {
	if a.env != nil {
		return *a.env
	}
	return storage.DefaultEnv()
}

func (a *App) storageOptions(ctx context.Context) storage.Options {
	return storage.Options{
		Env:      a.storageEnv(),
		Runner:   storage.OSRunner{},
		Admin:    system.IsElevated(),
		Progress: func(p storage.Progress) { a.emitEvent("storage:progress", p) },
	}
}

// beginStorageTask starts a long task: only one runs at a time, and it can be
// stopped with StorageCancel.
func (a *App) beginStorageTask() (context.Context, func(), error) {
	a.storage.mu.Lock()
	defer a.storage.mu.Unlock()
	if a.storage.busy {
		return nil, nil, errors.New("another storage task is still running")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.storage.busy, a.storage.cancel = true, cancel
	return ctx, func() {
		cancel()
		a.storage.mu.Lock()
		a.storage.busy, a.storage.cancel = false, nil
		a.storage.mu.Unlock()
	}, nil
}

// StorageVolumes lists the fixed drives with their free space.
func (a *App) StorageVolumes() []storage.Volume { return storage.Volumes() }

// StorageDefaults returns the folders the page offers to look in first.
func (a *App) StorageDefaults() storage.Defaults { return storage.DefaultFolders() }

// StorageScan measures every cleanup category without changing anything.
func (a *App) StorageScan() ([]storage.Category, error) {
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return nil, err
	}
	defer end()
	return storage.Scan(ctx, a.storageOptions(ctx))
}

// StorageClean removes the contents of the chosen categories. Ids the catalog
// of categories does not know are reported, never acted on.
func (a *App) StorageClean(ids []string) (storage.Result, error) {
	if len(ids) == 0 {
		return storage.Result{}, errors.New("nothing selected to clean")
	}
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return storage.Result{}, err
	}
	defer end()
	return storage.Clean(ctx, ids, a.storageOptions(ctx)), nil
}

func (a *App) remember(files []string, folders []string) {
	a.storage.mu.Lock()
	defer a.storage.mu.Unlock()
	if a.storage.listed == nil {
		a.storage.listed, a.storage.folders = map[string]bool{}, map[string]bool{}
	}
	for _, f := range files {
		a.storage.listed[pathKey(f)] = true
	}
	for _, d := range folders {
		a.storage.folders[pathKey(d)] = true
	}
}

// StorageBigFiles lists the largest files under a folder.
func (a *App) StorageBigFiles(root string, limit int) ([]storage.BigFile, error) {
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return nil, err
	}
	defer end()
	out, err := storage.BigFiles(ctx, root, limit, func(p storage.Progress) { a.emitEvent("storage:progress", p) })
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(out))
	for i, f := range out {
		paths[i] = f.Path
	}
	a.remember(paths, nil)
	return out, nil
}

// StorageFolders lists the folders directly under a folder, biggest first.
func (a *App) StorageFolders(root string) ([]storage.FolderSize, error) {
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return nil, err
	}
	defer end()
	out, err := storage.FolderSizes(ctx, root, func(p storage.Progress) { a.emitEvent("storage:progress", p) })
	if err != nil {
		return nil, err
	}
	dirs := make([]string, len(out))
	for i, f := range out {
		dirs[i] = f.Path
	}
	a.remember(nil, dirs)
	return out, nil
}

// StorageDuplicates finds files with identical content in the given folders.
func (a *App) StorageDuplicates(roots []string, minMB int) ([]storage.DupGroup, error) {
	if len(roots) == 0 {
		return nil, errors.New("choose at least one folder")
	}
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return nil, err
	}
	defer end()
	minMB = max(1, min(minMB, 10240))
	groups, err := storage.FindDuplicates(ctx, roots, int64(minMB)<<20, func(p storage.Progress) { a.emitEvent("storage:progress", p) })
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, g := range groups {
		for _, f := range g.Files {
			paths = append(paths, f.Path)
		}
	}
	a.remember(paths, nil)
	a.storage.mu.Lock()
	a.storage.groups = groups
	a.storage.mu.Unlock()
	return groups, nil
}

// StorageRecycle moves files to the Recycle Bin. Only files a scan listed are
// accepted, and a request that would remove every copy of a duplicated file is
// refused, whatever the interface sent.
func (a *App) StorageRecycle(paths []string) (storage.RecycleResult, error) {
	if len(paths) == 0 {
		return storage.RecycleResult{}, errors.New("no files selected")
	}
	a.storage.mu.Lock()
	want := map[string]bool{}
	for _, p := range paths {
		k := pathKey(p)
		if !a.storage.listed[k] {
			a.storage.mu.Unlock()
			return storage.RecycleResult{}, fmt.Errorf("%q was not listed by a scan, so it is not moved", p)
		}
		want[k] = true
	}
	for _, g := range a.storage.groups {
		left := 0
		for _, f := range g.Files {
			if !want[pathKey(f.Path)] {
				left++
			}
		}
		if left == 0 && anyWanted(g, want) {
			a.storage.mu.Unlock()
			return storage.RecycleResult{}, errors.New("that would remove every copy of a file: keep at least one")
		}
	}
	a.storage.mu.Unlock()

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	runner := a.runner
	if runner == nil {
		runner = storage.OSRunner{}
	}
	moved, freed, failed := storage.Recycle(ctx, runner, paths)
	if failed == nil {
		failed = []string{}
	}

	// Files that are gone no longer count as copies.
	failedSet := map[string]bool{}
	for _, f := range failed {
		failedSet[pathKey(f)] = true
	}
	a.storage.mu.Lock()
	var groups []storage.DupGroup
	for _, g := range a.storage.groups {
		var keep []storage.DupFile
		for _, f := range g.Files {
			if k := pathKey(f.Path); !want[k] || failedSet[k] {
				keep = append(keep, f)
			}
		}
		if len(keep) >= 2 {
			g.Files = keep
			groups = append(groups, g)
		} else if len(keep) == 1 {
			// The copy that is left is no longer listed, so a second request cannot
			// remove the last copy of a file that was a duplicate.
			delete(a.storage.listed, pathKey(keep[0].Path))
		}
	}
	a.storage.groups = groups
	for k := range want {
		if !failedSet[k] {
			delete(a.storage.listed, k)
		}
	}
	a.storage.mu.Unlock()
	return storage.RecycleResult{Moved: moved, FreedBytes: freed, Failed: failed}, nil
}

func anyWanted(g storage.DupGroup, want map[string]bool) bool {
	for _, f := range g.Files {
		if want[pathKey(f.Path)] {
			return true
		}
	}
	return false
}

// StorageReveal shows a listed file or folder in Explorer.
func (a *App) StorageReveal(path string) error {
	k := pathKey(path)
	a.storage.mu.Lock()
	ok := a.storage.listed[k] || a.storage.folders[k]
	a.storage.mu.Unlock()
	if !ok {
		return errors.New("that path was not listed by a scan")
	}
	// explorer.exe reports failure even when it opens, so its exit code is ignored.
	return exec.Command("explorer.exe", "/select,"+filepath.Clean(path)).Start()
}

// StorageCancel stops the scan or clean that is running, if any.
func (a *App) StorageCancel() {
	a.storage.mu.Lock()
	cancel := a.storage.cancel
	a.storage.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// StorageProjects looks for the dependency and build folders of projects nobody has worked on for minDays
// days or more, under the given folders. It changes nothing; the folders it lists are the only ones
// StorageProjectsClean will accept.
func (a *App) StorageProjects(roots []string, minDays int) ([]storage.ProjectJunk, error) {
	if len(roots) == 0 {
		return nil, errors.New("choose at least one folder")
	}
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return nil, err
	}
	defer end()
	minDays = max(1, min(minDays, 3650))
	out, err := storage.FindProjectJunk(ctx, roots, time.Duration(minDays)*24*time.Hour, a.clock(), func(p storage.Progress) { a.emitEvent("storage:progress", p) })
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(out))
	for i, j := range out {
		paths[i] = j.Path
	}
	a.storage.mu.Lock()
	a.storage.projects = map[string]bool{}
	a.storage.mu.Unlock()
	a.remember(nil, paths) // they can be shown in Explorer
	a.storage.mu.Lock()
	for _, p := range paths {
		a.storage.projects[pathKey(p)] = true
	}
	a.storage.mu.Unlock()
	return out, nil
}

// StorageProjectsClean deletes folders a project search listed. It cannot be undone, so it only accepts
// what that search returned, and each one is proved again at the moment it is removed.
func (a *App) StorageProjectsClean(paths []string) (storage.ProjectResult, error) {
	if len(paths) == 0 {
		return storage.ProjectResult{}, errors.New("nothing selected to remove")
	}
	a.storage.mu.Lock()
	for _, p := range paths {
		if !a.storage.projects[pathKey(p)] {
			a.storage.mu.Unlock()
			return storage.ProjectResult{}, fmt.Errorf("%q was not listed by a project search, so it is not removed", p)
		}
	}
	a.storage.mu.Unlock()
	ctx, end, err := a.beginStorageTask()
	if err != nil {
		return storage.ProjectResult{}, err
	}
	defer end()
	res := storage.RemoveProjectJunk(ctx, a.storageEnv(), paths, a.clock(), func(p storage.Progress) { a.emitEvent("storage:progress", p) })
	// What is gone is no longer listed; what was refused stays so the person can try again after fixing it.
	failed := map[string]bool{}
	for _, f := range res.Failed {
		failed[pathKey(f)] = true
	}
	a.storage.mu.Lock()
	for _, p := range paths {
		if !failed[pathKey(p)] {
			delete(a.storage.projects, pathKey(p))
		}
	}
	a.storage.mu.Unlock()
	return res, nil
}

// StorageVirtualDisks lists the disk images WSL and Docker Desktop keep. They can be shown in Explorer, and
// nothing else: they are never offered for removal, so they are not added to what can be moved or deleted.
func (a *App) StorageVirtualDisks() []storage.VirtualDisk {
	disks := storage.VirtualDisks(a.storageEnv())
	paths := make([]string, len(disks))
	for i, d := range disks {
		paths[i] = d.Path
	}
	a.remember(nil, paths) // reveal only
	return disks
}
