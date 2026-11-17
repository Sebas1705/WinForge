package privacy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Value is a registry value exactly as stored: whether it exists, its type and its raw bytes.
type Value struct {
	Exists bool
	Type   uint32
	Data   []byte
}

// System is the machine this package reads and writes: the current user's registry and the
// folder for backups. Tests replace it with an in-memory fake, so no test touches the registry.
// Paths are below HKEY_CURRENT_USER.
type System interface {
	// Get reads one value; a value or key that does not exist is Value{Exists: false} and no error.
	Get(path, name string) (Value, error)
	KeyExists(path string) (bool, error)
	// Set writes a value with the given type, creating the key when needed.
	Set(path, name string, typ uint32, data []byte) error
	// Delete removes a value. One that is already gone is fine.
	Delete(path, name string) error
	// DeleteKeyIfEmpty removes a key that has no values and no subkeys; anything else is left alone.
	DeleteKeyIfEmpty(path string) error
	// Broadcast tells running programs that settings changed.
	Broadcast() error
	BackupDir() (string, error)
}

// osSystem is the real machine. The registry methods live in system_windows.go (and stubs in
// system_other.go).
type osSystem struct{}

// NewSystem is the real current-user registry and the real app-data folder.
func NewSystem() System { return osSystem{} }

// BackupDir is %AppData%\WinForge\privacy-backups, next to WinForge's other files.
func (osSystem) BackupDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinForge", "privacy-backups"), nil
}

var errWindowsOnly = errors.New("privacy settings can only be read and changed on Windows")

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }
