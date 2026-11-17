package mcpserver

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/diag"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

// RealEnv fills the Env with this PC's real pieces: the embedded catalog (loaded once), a winget/registry
// snapshot, the OS executor, the elevation check, the lock shared with `winforge-cli apply` and `updates`,
// and a redactor that hides the user name and home folder the same way diagnostic reports do.
func RealEnv(version string) *mcpkit.Env {
	var (
		once sync.Once
		cat  *catalog.Catalog
		cerr error
	)
	sc := diag.NewScrubber(diag.Identity())
	return &mcpkit.Env{
		Version: version,
		Catalog: func() (*catalog.Catalog, error) {
			once.Do(func() { cat, cerr = catalog.Load(catalogdata.FS) })
			return cat, cerr
		},
		Snapshot: system.Snapshot,
		Exec:     install.OSExecutor{},
		Elevated: system.IsElevated,
		LockPath: func() (string, error) {
			dir, err := sched.DefaultDir()
			return filepath.Join(dir, "run.lock"), err
		},
		Now: time.Now,
		AppData: func() (string, error) {
			base, err := os.UserConfigDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(base, "WinForge"), nil
		},
		Redact: sc.Clean,
	}
}
