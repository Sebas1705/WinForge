//go:build windows

package services

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/Sebas1705/WinForge/internal/startup"
)

const servicesKey = `SYSTEM\CurrentControlSet\Services`

type osSystem struct{}

// NewSystem is the real machine: the services registry key for configuration, the service
// control manager for state, and sc.exe for the one change WinForge makes.
func NewSystem() System { return osSystem{} }

func (osSystem) Getenv(name string) string { return os.Getenv(name) }

func (osSystem) List() ([]Raw, error) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, servicesKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	out := make([]Raw, 0, len(names))
	for _, n := range names {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, servicesKey+`\`+n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		r := Raw{Name: n}
		if v, _, err := k.GetIntegerValue("Type"); err == nil {
			r.Type = uint32(v)
		} else {
			k.Close()
			continue // not a service or driver entry
		}
		if v, _, err := k.GetIntegerValue("Start"); err == nil {
			r.Start = uint32(v)
		} else {
			r.Start = 99 // unknown: never shown
		}
		if v, _, err := k.GetIntegerValue("DelayedAutostart"); err == nil {
			r.Delayed = v == 1
		}
		r.ImagePath, _, _ = k.GetStringValue("ImagePath")
		r.DisplayName, _, _ = k.GetStringValue("DisplayName")
		if v, _, err := k.GetStringsValue("DependOnService"); err == nil {
			r.DependsOn = v
		}
		k.Close()
		out = append(out, r)
	}
	return out, nil
}

func (osSystem) States(names []string) map[string]State {
	out := map[string]State{}
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return out
	}
	defer windows.CloseServiceHandle(scm)
	for _, n := range names {
		p, err := windows.UTF16PtrFromString(n)
		if err != nil {
			continue
		}
		h, err := windows.OpenService(scm, p, windows.SERVICE_QUERY_STATUS)
		if err != nil {
			continue
		}
		var st windows.SERVICE_STATUS
		if err := windows.QueryServiceStatus(h, &st); err == nil {
			switch st.CurrentState {
			case windows.SERVICE_RUNNING:
				out[n] = Running
			case windows.SERVICE_STOPPED:
				out[n] = Stopped
			default:
				out[n] = Other
			}
		}
		windows.CloseServiceHandle(h)
	}
	return out
}

func (osSystem) Publishers(paths []string) map[string]string {
	out := map[string]string{}
	for p, fi := range startup.NewSystem().Inspect(paths) {
		if fi.Publisher != "" {
			out[p] = fi.Publisher
		}
	}
	return out
}

// scValue is how sc.exe spells each mode.
var scValue = map[Mode]string{Auto: "auto", Delayed: "delayed-auto", Manual: "demand", Disabled: "disabled"}

// SetStartMode runs `sc.exe config <name> start= <mode>`: only the start type changes. The
// service is not stopped, started or deleted, and its recovery actions and dependencies are not
// touched. The name is passed as its own argument, never through a shell.
func (osSystem) SetStartMode(name string, m Mode) error {
	v, ok := scValue[m]
	if !ok || name == "" || strings.ContainsAny(name, `\/`) {
		return ErrBadMode
	}
	cmd := exec.Command("sc.exe", "config", name, "start=", v)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if out, err := cmd.CombinedOutput(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 5 {
			return ErrNeedsAdmin // ERROR_ACCESS_DENIED
		}
		return errors.New("svc:failed: " + strings.TrimSpace(string(out)))
	}
	if m == Auto {
		// Plain "auto" can leave the delayed flag set on some systems; the flag is cleared so the mode is what was asked.
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, servicesKey+`\`+name, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
			if d, _, err := k.GetIntegerValue("DelayedAutostart"); err == nil && d == 1 {
				_ = k.SetDWordValue("DelayedAutostart", 0)
			}
			k.Close()
		}
	}
	return nil
}
