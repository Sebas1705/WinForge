package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/patch"
)

// Emulation returns which systems each emulator plays and where free games,
// homebrew and patches for them are. WinForge only links to those places.
func (a *App) Emulation() catalog.Emulation {
	if a.cat.Emulation == nil {
		return catalog.Emulation{Systems: []catalog.System{}, Emulators: map[string][]string{}, Sources: []catalog.Source{}}
	}
	return *a.cat.Emulation
}

// PatchResult is what applying a patch produced.
type PatchResult struct {
	Path       string `json:"path"`
	Format     string `json:"format"`
	Checked    bool   `json:"checked"`
	Headerless bool   `json:"headerless"`
}

// PatchROM asks for the person's own game file and a patch (IPS, UPS or BPS),
// and writes the patched game next to the original. The original is never
// changed and nothing existing is overwritten. It returns an empty result when
// either dialog is cancelled.
func (a *App) PatchROM() (*PatchResult, error) {
	rom, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Your game file (the original, unpatched)"})
	if err != nil || rom == "" {
		return &PatchResult{}, err
	}
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "The patch",
		Filters: []runtime.FileFilter{{DisplayName: "Patches (*.ips, *.ups, *.bps)", Pattern: "*.ips;*.ups;*.bps"}},
	})
	if err != nil || p == "" {
		return &PatchResult{}, err
	}
	out, info, err := patch.ApplyFiles(rom, p)
	if err != nil {
		return nil, err
	}
	return &PatchResult{Path: out, Format: info.Format, Checked: info.Checked, Headerless: info.Headerless}, nil
}
