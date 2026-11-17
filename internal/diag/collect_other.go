//go:build !windows

package diag

import (
	"os/exec"
	"runtime"
)

func hide(*exec.Cmd) {}

// WindowsInfo is empty off Windows (WinForge only runs there; this keeps the package building).
func WindowsInfo() Windows { return Windows{Arch: runtime.GOARCH} }
