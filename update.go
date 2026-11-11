package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/updater"
)

// UpdateInfo is what the UI shows about a newer release.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
}

// CheckUpdate asks GitHub for the latest release. Development builds report
// no update rather than nagging.
func (a *App) CheckUpdate() (UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	rel, err := updater.New().Latest(ctx)
	if err != nil {
		return UpdateInfo{Current: version}, err
	}
	return UpdateInfo{
		Current:   version,
		Latest:    rel.Tag,
		Available: updater.Newer(version, rel.Tag),
		URL:       rel.URL,
		Notes:     rel.Notes,
	}, nil
}

// InstallUpdate downloads the latest installer, verifies its SHA-256 against
// the release's SHA256SUMS, launches it and quits so it can replace the app.
// Progress is streamed as "update:progress" events {done, total}.
func (a *App) InstallUpdate() error {
	u := updater.New()
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	rel, err := u.Latest(ctx)
	if err != nil {
		return err
	}
	if !updater.Newer(version, rel.Tag) {
		return errors.New("WinForge is already up to date")
	}
	// A fresh private directory per run: a fixed, predictable path under %TEMP%
	// could be pre-created or swapped by another local process. MkdirTemp makes
	// it owner-only (0700).
	dir, err := os.MkdirTemp("", "WinForge-update-")
	if err != nil {
		return err
	}
	last := time.Now()
	path, err := u.Download(ctx, rel, dir, func(done, total int64) {
		if time.Since(last) > 100*time.Millisecond || done == total {
			last = time.Now()
			runtime.EventsEmit(a.ctx, "update:progress", map[string]int64{"done": done, "total": total})
		}
	})
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	// The installer installs machine-wide and carries an elevation manifest,
	// so CreateProcess would fail; `start` goes through ShellExecute, which
	// shows the UAC prompt.
	if err := exec.Command("cmd", "/C", "start", "", path).Start(); err != nil {
		os.RemoveAll(dir)
		return err
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}
