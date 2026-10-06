//go:build !windows

package settings

import "os/exec"

// OSRunner runs real commands.
type OSRunner struct{}

// Run implements Runner.
func (OSRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
