//go:build windows

package envdoctor

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	userEnvKey    = `Environment`
	machineEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
)

// Env reads every string variable of one scope exactly as stored: a REG_EXPAND_SZ value keeps its
// %VAR% references. Values of other types are not environment variables Windows would use.
func (osSystem) Env(scope Scope) (map[string]Raw, error) {
	root, sub := registry.CURRENT_USER, userEnvKey
	if scope == Machine {
		root, sub = registry.LOCAL_MACHINE, machineEnvKey
	}
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return map[string]Raw{}, nil
		}
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Raw, len(names))
	for _, n := range names {
		if n == "" {
			continue
		}
		v, typ, err := k.GetStringValue(n)
		if err != nil {
			continue
		}
		out[n] = Raw{Value: v, Type: typ}
	}
	return out, nil
}

// SetUserVar writes one variable of HKCU\Environment with the given registry type.
func (osSystem) SetUserVar(name, value string, typ uint32) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if typ == TypeExpand {
		return k.SetExpandStringValue(name, value)
	}
	return k.SetStringValue(name, value)
}

// DeleteUserVar removes one variable of HKCU\Environment. A variable that is already gone is fine.
func (osSystem) DeleteUserVar(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, userEnvKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// Broadcast tells running programs (Explorer, so new terminals) that the environment changed.
func (osSystem) Broadcast() error {
	const (
		hwndBroadcast    = 0xFFFF
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
		broadcastTimeout = 5000 // ms per window
	)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	r, _, callErr := proc.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)),
		smtoAbortIfHung, broadcastTimeout, uintptr(unsafe.Pointer(&result)))
	if r == 0 {
		return callErr
	}
	return nil
}
