//go:build !windows

package privacy

// A machine with no Windows registry: this feature is Windows-only.
func (osSystem) Get(string, string) (Value, error)        { return Value{}, nil }
func (osSystem) KeyExists(string) (bool, error)           { return false, nil }
func (osSystem) Set(string, string, uint32, []byte) error { return errWindowsOnly }
func (osSystem) Delete(string, string) error              { return errWindowsOnly }
func (osSystem) DeleteKeyIfEmpty(string) error            { return errWindowsOnly }
func (osSystem) Broadcast() error                         { return nil }
