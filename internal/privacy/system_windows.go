//go:build windows

package privacy

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func (osSystem) Get(path, name string) (Value, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return Value{}, nil
		}
		return Value{}, err
	}
	defer k.Close()
	buf := make([]byte, 256)
	for {
		n, typ, err := k.GetValue(name, buf)
		switch {
		case err == nil:
			return Value{Exists: true, Type: typ, Data: append([]byte{}, buf[:n]...)}, nil
		case errors.Is(err, registry.ErrNotExist):
			return Value{}, nil
		case errors.Is(err, registry.ErrShortBuffer):
			buf = make([]byte, n+16)
		default:
			return Value{}, err
		}
	}
}

func (osSystem) KeyExists(path string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	k.Close()
	return true, nil
}

var regSetValueEx = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")

// Set writes the raw bytes with the exact type, so a value restored from a backup is bit for bit
// what was there (the registry package only offers typed setters).
func (osSystem) Set(path, name string, typ uint32, data []byte) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var p *byte
	if len(data) > 0 {
		p = &data[0]
	}
	r, _, _ := regSetValueEx.Call(uintptr(k), uintptr(unsafe.Pointer(n)), 0, uintptr(typ), uintptr(unsafe.Pointer(p)), uintptr(len(data)))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func (osSystem) Delete(path, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func (osSystem) DeleteKeyIfEmpty(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	st, err := k.Stat()
	k.Close()
	if err != nil {
		return err
	}
	if st.ValueCount != 0 || st.SubKeyCount != 0 {
		return nil
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// Broadcast tells Explorer and other running programs that policy and settings changed.
func (osSystem) Broadcast() error {
	const (
		hwndBroadcast    = 0xFFFF
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
		broadcastTimeout = 5000 // ms per window
	)
	what, err := windows.UTF16PtrFromString("Policy")
	if err != nil {
		return err
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	r, _, callErr := proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(what)),
		smtoAbortIfHung, broadcastTimeout, uintptr(unsafe.Pointer(&result)))
	if r == 0 {
		return callErr
	}
	return nil
}
