//go:build windows

package settings

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// OSRunner runs real commands, hidden and time-limited.
type OSRunner struct{}

// Run implements Runner.
func (OSRunner) Run(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Output()
}
