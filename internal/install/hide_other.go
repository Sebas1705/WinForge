//go:build !windows

package install

import "os/exec"

func hide(*exec.Cmd) {}
