//go:build windows

package diag

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// WindowsInfo reads the Windows version, build, edition and the architecture WinForge runs as.
func WindowsInfo() Windows {
	w := Windows{Arch: runtime.GOARCH}
	v := windows.RtlGetVersion()
	w.Version = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	w.Build = strconv.FormatUint(uint64(v.BuildNumber), 10)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return w
	}
	defer k.Close()
	if s, _, err := k.GetStringValue("EditionID"); err == nil {
		w.Edition = s
	}
	if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
		w.Build = fmt.Sprintf("%d.%d", v.BuildNumber, ubr)
	}
	return w
}
