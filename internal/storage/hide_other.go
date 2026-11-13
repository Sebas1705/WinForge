//go:build !windows

package storage

import "os/exec"

func hide(*exec.Cmd) {}
