package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/health"
)

// HealthFinding is a finding plus, when a catalog app helps with it, that
// app's catalog id (the finding itself only knows the winget id).
type HealthFinding struct {
	Key       string            `json:"key"`
	Group     string            `json:"group"`
	Severity  string            `json:"severity"`
	Params    map[string]string `json:"params,omitempty"`
	Links     []health.Link     `json:"links,omitempty"`
	Winget    string            `json:"winget,omitempty"`
	CatalogID string            `json:"catalogId,omitempty"`
}

// HealthResult is everything the PC health page shows.
type HealthResult struct {
	Report   *health.Report     `json:"report"`
	Findings []HealthFinding    `json:"findings"`
	Updates  *health.UpdateScan `json:"updates"`
	OK       int                `json:"ok"`
	Total    int                `json:"total"`
}

// HealthScan reads firmware, drivers, storage and security. It only reads.
func (a *App) HealthScan() (HealthResult, error) {
	r, err := health.CollectWith(a.ctx, func(s string) { a.scanStep("health", s) })
	if err != nil {
		return HealthResult{}, err
	}
	a.mu.Lock()
	a.healthReport = r
	a.mu.Unlock()
	return a.healthResult(), nil
}

// HealthUpdates searches Windows Update for pending drivers, firmware and
// updates. It never downloads or installs anything.
func (a *App) HealthUpdates() (HealthResult, error) {
	a.mu.Lock()
	have := a.healthReport != nil
	a.mu.Unlock()
	if !have {
		if _, err := a.HealthScan(); err != nil {
			return HealthResult{}, err
		}
	}
	scan, err := health.ScanUpdates(a.ctx)
	if err != nil {
		return HealthResult{}, err
	}
	a.mu.Lock()
	a.healthUpdates = scan
	a.mu.Unlock()
	return a.healthResult(), nil
}

func (a *App) healthResult() HealthResult {
	a.mu.Lock()
	r, scan := a.healthReport, a.healthUpdates
	a.mu.Unlock()
	fs := health.Analyze(r, scan, time.Now())
	byWinget := map[string]string{}
	for id, app := range a.cat.Apps {
		byWinget[strings.ToLower(app.Winget)] = id
	}
	out := make([]HealthFinding, 0, len(fs))
	for _, f := range fs {
		hf := HealthFinding{Key: f.Key, Group: string(f.Group), Severity: string(f.Severity), Params: f.Params, Links: f.Links, Winget: f.Winget}
		if f.Winget != "" {
			hf.CatalogID = byWinget[strings.ToLower(f.Winget)]
		}
		out = append(out, hf)
	}
	ok, total := health.Tally(fs)
	return HealthResult{Report: r, Findings: out, Updates: scan, OK: ok, Total: total}
}

// SaveTextFile asks where to save and writes content, for exporting a report.
// It returns the chosen path, or "" if the dialog was cancelled.
func (a *App) SaveTextFile(name, content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: "Markdown (*.md)", Pattern: "*.md"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(content), 0o644)
}

// OpenLink opens a vendor page in the browser, or a Windows settings page.
// Only https links and two known Windows URI schemes are allowed; the
// frontend never gets to launch anything else.
func (a *App) OpenLink(u string) error {
	switch {
	case strings.HasPrefix(u, "https://"):
		runtime.BrowserOpenURL(a.ctx, u)
		return nil
	case u == "windowsdefender:" || strings.HasPrefix(u, "ms-settings:windowsupdate"):
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		// `start` resolves the URI scheme through ShellExecute.
		return exec.CommandContext(ctx, "cmd", "/C", "start", "", u).Run()
	}
	return errors.New("link not allowed")
}
