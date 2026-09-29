//go:build windows

package system

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the process runs with an administrator token.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RestartElevated relaunches the executable through the UAC prompt. The caller
// should quit after it returns nil.
func RestartElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString(strings.Join(os.Args[1:], " "))
	return windows.ShellExecute(0, verb, file, args, nil, windows.SW_NORMAL)
}
