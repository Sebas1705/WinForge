//go:build !windows

package uninstall

// DefaultEnv is an empty layout: the leftovers search is Windows-only, so off Windows it has no places to
// read and finds nothing.
func DefaultEnv() Env { return Env{} }

// registryApps: there is no list of installed programs off Windows.
func registryApps() []RegApp { return nil }
