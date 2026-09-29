package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// exportFile is the subset of `winget export` output WinForge reads.
type exportFile struct {
	Sources []struct {
		Packages []struct {
			PackageIdentifier string `json:"PackageIdentifier"`
			Version           string `json:"Version"`
		} `json:"Packages"`
	} `json:"Sources"`
}

// ParseWingetExport turns `winget export --include-versions` JSON into a map
// of lower-cased PackageIdentifier to version.
func ParseWingetExport(b []byte) (map[string]string, error) {
	var f exportFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse winget export: %w", err)
	}
	out := map[string]string{}
	for _, s := range f.Sources {
		for _, p := range s.Packages {
			out[strings.ToLower(p.PackageIdentifier)] = p.Version
		}
	}
	return out, nil
}

// ErrNoWinget is returned when winget.exe is not on PATH.
var ErrNoWinget = errors.New("winget is not available; install App Installer from the Microsoft Store")

// WingetInventory asks winget what it knows is installed. The JSON export is
// used instead of scraping `winget list` because that table is localized and
// truncates columns.
func WingetInventory(ctx context.Context) (map[string]string, error) {
	if _, err := exec.LookPath("winget"); err != nil {
		return nil, ErrNoWinget
	}
	tmp, err := os.CreateTemp("", "winforge-export-*.json")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.CommandContext(ctx, "winget", "export", "-o", tmp.Name(),
		"--include-versions", "--accept-source-agreements", "--disable-interactivity")
	hideWindow(cmd)
	// winget export exits non-zero when some installed packages have no
	// source; the file is still written, so only a missing file is an error.
	_ = cmd.Run()
	b, err := os.ReadFile(tmp.Name())
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("winget export produced no output: %v", err)
	}
	return ParseWingetExport(b)
}

// Snapshot takes a full inventory of the machine. Winget being unavailable is
// not fatal: registry, PATH and file detection still work.
func Snapshot(ctx context.Context) (Inventory, error) {
	inv := Inventory{LookPath: LookPath, Exists: Exists, DisplayNames: RegistryDisplayNames()}
	w, err := WingetInventory(ctx)
	if err != nil {
		inv.Winget = map[string]string{}
		return inv, err
	}
	inv.Winget = w
	return inv, nil
}
