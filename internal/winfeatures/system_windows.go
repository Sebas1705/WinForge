//go:build windows

package winfeatures

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

type osSystem struct{}

// NewSystem is the real machine: CIM (Win32_OptionalFeature) through PowerShell, wsl.exe and the
// registry. Everything it does is read-only.
func NewSystem() System { return osSystem{} }

// featureNames are the only features asked about, so the script never builds a query from input.
func featureNames() string {
	var q []string
	for _, k := range known {
		q = append(q, "'"+k.windows+"'")
	}
	return strings.Join(q, ",")
}

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func powershell(script string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	hide(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return out.String(), fmt.Errorf("%w: %s", err, msg)
		}
		return out.String(), err
	}
	return out.String(), nil
}

// OptionalFeatures reads Win32_OptionalFeature, which an ordinary user may read.
func (osSystem) OptionalFeatures() (map[string]int, error) {
	out, err := powershell(`$ErrorActionPreference='Stop'; Get-CimInstance Win32_OptionalFeature | Where-Object { $_.Name -in `+featureNames()+` } | ForEach-Object { $_.Name + '=' + $_.InstallState }`, 60*time.Second)
	if err != nil {
		return nil, err
	}
	states := map[string]int{}
	for _, l := range strings.Split(out, "\n") {
		name, val, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(val); err == nil {
			states[name] = n
		}
	}
	// Windows lists every feature it has, so an empty answer means the query failed quietly, not
	// that a whole edition has none of them (WSL and .NET 3.5 exist on every edition).
	if len(states) == 0 {
		return nil, errors.New("Windows returned no optional features")
	}
	return states, nil
}

// PendingFeatures asks Get-WindowsOptionalFeature, which needs administrator rights.
func (osSystem) PendingFeatures() (map[string]string, error) {
	out, err := powershell(`$ErrorActionPreference='Stop'; Get-WindowsOptionalFeature -Online -FeatureName `+featureNames()+` -ErrorAction SilentlyContinue | ForEach-Object { $_.FeatureName + '=' + $_.State }`, 90*time.Second)
	if err != nil {
		return nil, err
	}
	pending := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		name, val, ok := strings.Cut(strings.TrimSpace(l), "=")
		switch {
		case ok && strings.EqualFold(val, "EnablePending"):
			pending[name] = PendingEnable
		case ok && strings.EqualFold(val, "DisablePending"):
			pending[name] = PendingDisable
		}
	}
	return pending, nil
}

// WSL runs wsl.exe for a listing. It is never given anything that starts or enters a distribution.
func (osSystem) WSL(args ...string) ([]byte, int, error) {
	path, err := exec.LookPath("wsl.exe")
	if err != nil {
		return nil, 0, ErrWSLMissing
	}
	for _, a := range args {
		if a != "-l" && a != "-v" && a != "-q" && a != "--running" {
			return nil, 0, errors.New("winfeatures: only wsl listing arguments are allowed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	hide(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), ee.ExitCode(), nil
	}
	if err != nil {
		return nil, 0, err
	}
	return out.Bytes(), 0, nil
}

// WSLDefaultVersion reads the version `wsl --set-default-version` stored for this user.
func (osSystem) WSLDefaultVersion() (int, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("DefaultVersion")
	if err != nil || v < 1 || v > 2 {
		return 0, false
	}
	return int(v), true
}
