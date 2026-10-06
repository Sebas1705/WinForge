//go:build !windows

package health

import "os/exec"

func hide(*exec.Cmd) {}
