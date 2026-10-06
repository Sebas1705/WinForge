//go:build windows

package settings

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

// OSRunner runs real commands, hidden and time-limited.
type OSRunner struct{}

func regPath(root registry.Key, sub string) string {
	k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	if exp, err := registry.ExpandString(v); err == nil {
		return exp
	}
	return v
}

// freshPath is the PATH Windows would give a new program now. WinForge started
// before the apps it just installed, so its own PATH does not list them.
func freshPath() string {
	machine := regPath(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	user := regPath(registry.CURRENT_USER, `Environment`)
	return strings.Trim(machine+";"+user+";"+os.Getenv("PATH"), ";")
}

// lookIn finds an executable by name in a PATH string.
func lookIn(path, name string) string {
	exts := []string{".exe", ".cmd", ".bat", ".com"}
	if filepath.Ext(name) != "" {
		exts = []string{""}
	}
	for _, dir := range filepath.SplitList(path) {
		for _, e := range exts {
			p := filepath.Join(dir, name+e)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// Run implements Runner.
func (OSRunner) Run(name string, args ...string) ([]byte, error) {
	path := freshPath()
	exe := lookIn(path, name)
	if exe == "" {
		return nil, exec.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = append(os.Environ(), "PATH="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Output()
}
