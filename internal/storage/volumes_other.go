//go:build !windows

package storage

// Volumes is empty off Windows: drive letters and the fixed-drive query are
// Windows concepts, and only the pure logic of this package runs elsewhere.
func Volumes() []Volume { return []Volume{} }

func platformDefaults(*Defaults) {}
