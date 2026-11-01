//go:build windows

package install

import (
	"os/exec"
	"strconv"
	"syscall"
)

// killTree makes cancelling kill the whole process tree. Killing only the
// process we started leaves its children running (and holding our output
// pipe) - powershell -> sfc, winget -> an installer. taskkill /T walks the
// tree; the plain Kill afterwards is the fallback and reports the result.
func killTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = tk.Run()
		return cmd.Process.Kill()
	}
}
