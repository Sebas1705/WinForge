//go:build windows

package storage

import (
	"golang.org/x/sys/windows"
)

// Volumes lists the fixed drives with their size and free space.
func Volumes() []Volume {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return []Volume{}
	}
	out := []Volume{}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		letter := string(rune('A'+i)) + ":"
		root, err := windows.UTF16PtrFromString(letter + `\`)
		if err != nil || windows.GetDriveType(root) != windows.DRIVE_FIXED {
			continue
		}
		var free, total, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(root, &free, &total, &totalFree); err != nil {
			continue
		}
		label := make([]uint16, 261)
		_ = windows.GetVolumeInformation(root, &label[0], uint32(len(label)), nil, nil, nil, nil, 0)
		out = append(out, Volume{Letter: letter, Label: windows.UTF16ToString(label), TotalBytes: int64(total), FreeBytes: int64(free)})
	}
	return out
}

// knownFolder returns a Windows known folder (Downloads, Pictures...), which
// follows the person's moves and OneDrive redirection, or "" if unknown.
func knownFolder(id *windows.KNOWNFOLDERID) string {
	p, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return ""
	}
	return p
}

func platformDefaults(d *Defaults) {
	if p := knownFolder(windows.FOLDERID_Downloads); p != "" {
		d.Downloads = p
	}
	if p := knownFolder(windows.FOLDERID_Documents); p != "" {
		d.Documents = p
	}
	if p := knownFolder(windows.FOLDERID_Pictures); p != "" {
		d.Pictures = p
	}
	if p := knownFolder(windows.FOLDERID_Videos); p != "" {
		d.Videos = p
	}
}
