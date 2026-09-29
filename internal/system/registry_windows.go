//go:build windows

package system

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// RegistryDisplayNames returns the DisplayName of every Uninstall entry in the
// machine (both bitnesses) and current-user hives, which is what Windows'
// "Installed apps" list is built from.
func RegistryDisplayNames() []string {
	const path = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	const wow = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`
	var names []string
	for _, loc := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, path},
		{registry.LOCAL_MACHINE, wow},
		{registry.CURRENT_USER, path},
	} {
		k, err := registry.OpenKey(loc.root, loc.path, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			sk, err := registry.OpenKey(loc.root, loc.path+`\`+s, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			if n, _, err := sk.GetStringValue("DisplayName"); err == nil && n != "" {
				names = append(names, n)
			}
			sk.Close()
		}
	}
	return names
}
