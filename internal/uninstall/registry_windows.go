//go:build windows

package uninstall

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// DefaultEnv reads this PC's folders from the environment.
func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	return Env{
		Home:            home,
		AppData:         os.Getenv("APPDATA"),
		LocalAppData:    os.Getenv("LOCALAPPDATA"),
		ProgramData:     os.Getenv("ProgramData"),
		ProgramFiles:    os.Getenv("ProgramFiles"),
		ProgramFilesX86: os.Getenv("ProgramFiles(x86)"),
		WinDir:          os.Getenv("SystemRoot"),
		SystemDrive:     drive,
		Public:          os.Getenv("PUBLIC"),
		OneDrive:        os.Getenv("OneDrive"),
	}
}

// registryApps reads the DisplayName and EstimatedSize of every Uninstall entry, in the machine (both
// bitnesses) and current-user hives: what Windows' "Installed apps" list is built from.
func registryApps() []RegApp {
	const path = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	const wow = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`
	var out []RegApp
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
				app := RegApp{Name: n}
				if v, _, err := sk.GetIntegerValue("EstimatedSize"); err == nil {
					app.SizeKB = int64(v)
				}
				out = append(out, app)
			}
			sk.Close()
		}
	}
	return out
}
