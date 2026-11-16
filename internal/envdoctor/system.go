package envdoctor

import (
	"os"
	"path/filepath"
)

// osSystem is the real machine. The registry and broadcast methods live in system_windows.go
// (and a stub in system_other.go); the plain file and environment reads are the same everywhere.
type osSystem struct{}

// NewSystem is the real machine: the registry, the file system and the process environment.
func NewSystem() System { return osSystem{} }

func (osSystem) Getenv(name string) string { return os.Getenv(name) }

// Stat follows symbolic links and junctions. An App execution alias is a reparse point that
// os.Stat cannot open, so a path that Lstat sees counts as a file.
func (osSystem) Stat(p string) PathKind {
	st, err := os.Stat(p)
	if err != nil {
		lst, lerr := os.Lstat(p)
		if lerr != nil || lst.IsDir() {
			return Missing
		}
		return File
	}
	if st.IsDir() {
		return Dir
	}
	return File
}

// BackupDir is %AppData%\WinForge\env-backups, next to the rest of WinForge's own files.
func (osSystem) BackupDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinForge", "env-backups"), nil
}
