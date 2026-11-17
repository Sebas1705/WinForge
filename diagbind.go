package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/diag"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

// diagSession is the Settings page's diagnostic report. The text shown in the preview is kept, so
// what is saved is exactly what the person saw. env and pick are replaced by tests (nil means this
// PC and the real Save dialog).
type diagSession struct {
	mu   sync.Mutex
	text string
	env  func() diag.Input
	pick func(defaultName string) (string, error)
}

// DiagPreview builds the diagnostic report and returns its exact text. Nothing is written or sent.
func (a *App) DiagPreview() (string, error) {
	in := a.diagInput()
	text := diag.Render(in)
	a.diag.mu.Lock()
	a.diag.text = text
	a.diag.mu.Unlock()
	return text, nil
}

// DiagSave writes the report the last DiagPreview showed (building it first when there was none) to
// path. An empty path asks the person where to save it; the empty string comes back when they cancel.
// A path must be an absolute .json or .txt file, so this cannot be used to write anything else.
func (a *App) DiagSave(path string) (string, error) {
	a.diag.mu.Lock()
	text := a.diag.text
	a.diag.mu.Unlock()
	if text == "" {
		var err error
		if text, err = a.DiagPreview(); err != nil {
			return "", err
		}
	}
	if path == "" {
		var err error
		pick := a.diag.pick
		if pick == nil {
			pick = a.pickDiagPath
		}
		if path, err = pick("winforge-diagnostics.json"); err != nil || path == "" {
			return "", err
		}
	}
	if err := checkDiagPath(path); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(text), 0o644)
}

func (a *App) pickDiagPath(name string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: "WinForge diagnostic report (*.json)", Pattern: "*.json;*.txt"}},
	})
}

var errDiagPath = errors.New("diag:path")

func checkDiagPath(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !filepath.IsAbs(path) || (ext != ".json" && ext != ".txt") {
		return errDiagPath
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return errDiagPath
	}
	return nil
}

// diagInput reads what the report is made of from this PC and this session.
func (a *App) diagInput() diag.Input {
	if a.diag.env != nil {
		return a.diag.env()
	}
	users, homes := diag.Identity()
	in := diag.Input{
		Version:         version,
		Build:           diag.BuildInfo(),
		Windows:         diag.WindowsInfo(),
		Admin:           system.IsElevated(),
		WingetVersion:   diag.WingetVersion(context.Background()),
		CatalogApps:     len(a.cat.Apps),
		CatalogProfiles: len(a.cat.Profiles),
		Users:           users,
		Homes:           homes,
		Scans:           a.diagScans(),
	}
	if base, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(base, "WinForge")
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			in.DataFS, in.DataDir = os.DirFS(dir), dir
		}
		// The language the interface last told the scheduled task about, else Windows' own.
		in.Language = sched.Language(filepath.Join(dir, "auto-updates"), sched.SystemLanguage)
	}
	return in
}

// diagScans summarises the scans of this session as counts. It never names what was found.
func (a *App) diagScans() []diag.Scan {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []diag.Scan
	if a.installed != nil {
		out = append(out, diag.Scan{Name: "apps", Counts: map[string]int{"installed": len(a.installed), "catalog": len(a.cat.Apps)}})
	}
	if a.upgrades != nil {
		out = append(out, diag.Scan{Name: "updates", Counts: map[string]int{"available": len(a.upgrades)}})
	}
	if a.healthReport != nil {
		findings := health.Analyze(a.healthReport, a.healthUpdates, a.clock())
		ok, total := health.Tally(findings)
		out = append(out, diag.Scan{Name: "health", Counts: map[string]int{"fine": ok, "checks": total}})
	}
	return out
}
