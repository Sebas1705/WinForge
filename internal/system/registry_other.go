//go:build !windows

package system

import "os/exec"

func hideWindow(*exec.Cmd) {}

// RegistryDisplayNames is empty off Windows, so tests and CI on other
// platforms still build.
func RegistryDisplayNames() []string { return nil }
