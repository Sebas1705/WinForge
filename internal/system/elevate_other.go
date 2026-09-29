//go:build !windows

package system

import "errors"

// IsElevated is always false off Windows.
func IsElevated() bool { return false }

// RestartElevated is unsupported off Windows.
func RestartElevated() error { return errors.New("elevation is only supported on Windows") }
