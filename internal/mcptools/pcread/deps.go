package pcread

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/healthhistory"
	"github.com/Sebas1705/WinForge/internal/privacy"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/tasks"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// Deps are the backends the tools call, one small func each, so tests replace them with fakes that record
// their calls. Every real implementation below only reads.
type Deps struct {
	HealthCollect func(ctx context.Context, onStep func(string)) (*health.Report, error)
	HealthUpdates func(ctx context.Context) (*health.UpdateScan, error)
	HealthHistory func(ctx context.Context) (healthhistory.View, error)
	Battery       func(ctx context.Context) (battery.Report, error)

	Volumes      func() []storage.Volume
	StorageScan  func(ctx context.Context, admin bool, progress storage.ProgressFunc) ([]storage.Category, error)
	BigFiles     func(ctx context.Context, root string, limit int, progress storage.ProgressFunc) ([]storage.BigFile, error)
	FolderSizes  func(ctx context.Context, root string, progress storage.ProgressFunc) ([]storage.FolderSize, error)
	ProjectJunk  func(ctx context.Context, roots []string, minAge time.Duration, now time.Time, progress storage.ProgressFunc) ([]storage.ProjectJunk, error)
	VirtualDisks func() []storage.VirtualDisk
	CheckFolder  func(path string) error // nil when path is an existing folder
	StartupList  func() ([]startup.Item, error)
	ServicesList func() ([]services.Item, error)
	TasksList    func() ([]tasks.Item, error)
	EnvScan      func() (envdoctor.Report, error)
	FeaturesList func(elevated bool) []winfeatures.Feature
	WSLInfo      func() winfeatures.WSLInfo
	PrivacyList  func() ([]privacy.TweakView, error)
}

// RealDeps reads this PC.
func RealDeps() Deps {
	return Deps{
		HealthCollect: health.CollectWith,
		HealthUpdates: health.ScanUpdates,
		HealthHistory: readHealthHistory,
		Battery:       battery.Read,
		Volumes:       storage.Volumes,
		StorageScan: func(ctx context.Context, admin bool, progress storage.ProgressFunc) ([]storage.Category, error) {
			return storage.Scan(ctx, storage.Options{Env: storage.DefaultEnv(), Runner: readOnlyRunner{}, Admin: admin, Progress: progress})
		},
		BigFiles:    storage.BigFiles,
		FolderSizes: storage.FolderSizes,
		ProjectJunk: storage.FindProjectJunk,
		VirtualDisks: func() []storage.VirtualDisk {
			return storage.VirtualDisks(storage.DefaultEnv())
		},
		CheckFolder: func(p string) error {
			st, err := os.Stat(p)
			if err != nil {
				return errors.New("that folder does not exist or cannot be read")
			}
			if !st.IsDir() {
				return errors.New("that path is not a folder")
			}
			return nil
		},
		StartupList: func() ([]startup.Item, error) {
			return (&startup.Manager{Sys: startup.NewSystem()}).List()
		},
		// A Manager without a Store never writes the "original state" file: List only reads.
		ServicesList: func() ([]services.Item, error) {
			return (&services.Manager{Sys: services.NewSystem()}).List()
		},
		TasksList: func() ([]tasks.Item, error) {
			return (&tasks.Manager{Sys: tasks.NewSystem()}).List()
		},
		EnvScan: func() (envdoctor.Report, error) {
			return (&envdoctor.Manager{Sys: envdoctor.NewSystem()}).Scan()
		},
		FeaturesList: func(elevated bool) []winfeatures.Feature {
			return winfeatures.List(winfeatures.NewSystem(), elevated)
		},
		WSLInfo: func() winfeatures.WSLInfo { return winfeatures.WSL(winfeatures.NewSystem()) },
		PrivacyList: func() ([]privacy.TweakView, error) {
			return (&privacy.Manager{Sys: privacy.NewSystem()}).List()
		},
	}
}

// withDefaults fills every nil func with the real one.
func (d Deps) withDefaults() Deps {
	r := RealDeps()
	if d.HealthCollect == nil {
		d.HealthCollect = r.HealthCollect
	}
	if d.HealthUpdates == nil {
		d.HealthUpdates = r.HealthUpdates
	}
	if d.HealthHistory == nil {
		d.HealthHistory = r.HealthHistory
	}
	if d.Battery == nil {
		d.Battery = r.Battery
	}
	if d.Volumes == nil {
		d.Volumes = r.Volumes
	}
	if d.StorageScan == nil {
		d.StorageScan = r.StorageScan
	}
	if d.BigFiles == nil {
		d.BigFiles = r.BigFiles
	}
	if d.FolderSizes == nil {
		d.FolderSizes = r.FolderSizes
	}
	if d.ProjectJunk == nil {
		d.ProjectJunk = r.ProjectJunk
	}
	if d.VirtualDisks == nil {
		d.VirtualDisks = r.VirtualDisks
	}
	if d.CheckFolder == nil {
		d.CheckFolder = r.CheckFolder
	}
	if d.StartupList == nil {
		d.StartupList = r.StartupList
	}
	if d.ServicesList == nil {
		d.ServicesList = r.ServicesList
	}
	if d.TasksList == nil {
		d.TasksList = r.TasksList
	}
	if d.EnvScan == nil {
		d.EnvScan = r.EnvScan
	}
	if d.FeaturesList == nil {
		d.FeaturesList = r.FeaturesList
	}
	if d.WSLInfo == nil {
		d.WSLInfo = r.WSLInfo
	}
	if d.PrivacyList == nil {
		d.PrivacyList = r.PrivacyList
	}
	return d
}

// readHealthHistory reads the history file without going through FileStore.Load, which renames a damaged
// file out of the way: a read-only tool must not touch the file. A damaged or newer file reads as empty.
func readHealthHistory(ctx context.Context) (healthhistory.View, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return healthhistory.View{}, err
	}
	return readHistoryFile(filepath.Join(base, "WinForge", "health-history.json"))
}

func readHistoryFile(path string) (healthhistory.View, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return healthhistory.BuildView(healthhistory.File{}), nil
	}
	if err != nil {
		return healthhistory.View{}, err
	}
	var f healthhistory.File
	if json.Unmarshal(b, &f) != nil || f.Version > 1 {
		return healthhistory.BuildView(healthhistory.File{}), nil
	}
	return healthhistory.BuildView(f), nil
}

// allowedCommand says whether the storage scan may run a command: only the two read commands it uses to size
// Docker's build cache and dangling images.
func allowedCommand(name string, args []string) bool {
	return name == "docker" && len(args) >= 2 &&
		((args[0] == "system" && args[1] == "df") || (args[0] == "images" && args[1] == "--filter"))
}

// readOnlyRunner refuses everything the storage scan does not need, in particular the commands a clean runs.
type readOnlyRunner struct{}

func (readOnlyRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !allowedCommand(name, args) {
		return nil, errors.New("pcread: command refused: read-only tools do not run commands")
	}
	return storage.OSRunner{}.Run(ctx, name, args...)
}
