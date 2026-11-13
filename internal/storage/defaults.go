package storage

import (
	"os"
	"path/filepath"
)

// Volume is a fixed drive.
type Volume struct {
	Letter     string `json:"letter"` // "C:"
	Label      string `json:"label"`
	TotalBytes int64  `json:"totalBytes"`
	FreeBytes  int64  `json:"freeBytes"`
}

// Defaults are the folders the interface offers to look in first.
type Defaults struct {
	Home      string `json:"home"`
	Downloads string `json:"downloads"`
	Documents string `json:"documents"`
	Pictures  string `json:"pictures"`
	Videos    string `json:"videos"`
}

// DefaultFolders returns the person's folders. Windows known folders are used
// when available, so a Documents folder moved to OneDrive or another drive is
// found; otherwise the usual names under the profile folder.
func DefaultFolders() Defaults {
	home, _ := os.UserHomeDir()
	d := Defaults{
		Home:      home,
		Downloads: filepath.Join(home, "Downloads"),
		Documents: filepath.Join(home, "Documents"),
		Pictures:  filepath.Join(home, "Pictures"),
		Videos:    filepath.Join(home, "Videos"),
	}
	platformDefaults(&d)
	return d
}
