//go:build windows

package battery

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// runPowercfg writes the report with `powercfg /batteryreport`. It changes no setting.
func runPowercfg(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "powercfg", "/batteryreport", "/output", path, "/xml")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("powercfg: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
