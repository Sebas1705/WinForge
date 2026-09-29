//go:build windows

package install

import (
	"os/exec"
	"syscall"
)

// hide keeps a console window from flashing for every child process.
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
