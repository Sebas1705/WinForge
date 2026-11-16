//go:build !windows

package envdoctor

import "errors"

var errWindowsOnly = errors.New("environment variables can only be read and changed on Windows")

// A machine with no Windows environment: this feature is Windows-only.
func (osSystem) Env(Scope) (map[string]Raw, error)       { return map[string]Raw{}, nil }
func (osSystem) SetUserVar(string, string, uint32) error { return errWindowsOnly }
func (osSystem) DeleteUserVar(string) error              { return errWindowsOnly }
func (osSystem) Broadcast() error                        { return nil }
