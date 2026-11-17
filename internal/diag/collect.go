package diag

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// WingetVersion asks winget for its version ("" when winget is not installed or does not answer
// within a few seconds). It runs `winget --version` and nothing else.
func WingetVersion(ctx context.Context) string {
	path, err := exec.LookPath("winget")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// BuildInfo describes how this program was built: the Go version, platform and, when the build
// recorded it, the short commit.
func BuildInfo() string {
	s := runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
	if bi, ok := debug.ReadBuildInfo(); ok {
		rev, dirty := "", ""
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				rev = kv.Value
			case "vcs.modified":
				if kv.Value == "true" {
					dirty = "+dirty"
				}
			}
		}
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if rev != "" {
			s += " " + rev + dirty
		}
	}
	return s
}

// Identity is what must be scrubbed from the report: this account's user names and profile
// folders. They are read only to be removed; nothing here is ever written into the report.
func Identity() (users, homes []string) {
	users = append(users, os.Getenv("USERNAME"), os.Getenv("USER"))
	homes = append(homes, os.Getenv("USERPROFILE"), os.Getenv("HOME"))
	if h, err := os.UserHomeDir(); err == nil {
		homes = append(homes, h)
	}
	if u, err := user.Current(); err == nil {
		users = append(users, u.Username, u.Name)
		homes = append(homes, u.HomeDir)
	}
	return users, homes
}
