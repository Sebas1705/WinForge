//go:build windows

package offlinekit

import (
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// isReparse reports a junction or any other reparse point, which Windows lets a folder be without being a
// symbolic link.
func isReparse(fi os.FileInfo) bool {
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

// freeBytes is the space available to this person on the drive (or share) that holds path.
func freeBytes(path string) (int64, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	// GetDiskFreeSpaceEx wants a folder; walk up to one that exists (the kit folder may not exist yet).
	for {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	p, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(free), nil
}
