//go:build !windows

package offlinekit

import (
	"errors"
	"os"
)

// isReparse: only Windows has reparse points; links elsewhere are symbolic links, which Mode reports.
func isReparse(os.FileInfo) bool { return false }

// freeBytes: offline kits are a Windows feature; off Windows the free space is unknown.
func freeBytes(string) (int64, error) { return 0, errors.New("free space is only read on Windows") }
