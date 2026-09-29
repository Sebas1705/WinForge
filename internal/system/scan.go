// Package system inspects the PC: which catalog apps are installed and at
// which version. Detection combines three signals because no single one is
// complete: winget's own inventory, the Uninstall registry keys and PATH.
package system

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// Installed is what is known about one installed catalog app.
type Installed struct {
	ID      string   `json:"id"`
	Version string   `json:"version,omitempty"`
	Sources []string `json:"sources"` // winget, registry, path, file
}

// Inventory is a snapshot of the machine, decoupled from how it was taken so
// detection logic can be tested without a Windows PC.
type Inventory struct {
	// Winget maps lower-cased PackageIdentifier to version.
	Winget map[string]string
	// DisplayNames are the DisplayName values of Uninstall registry entries.
	DisplayNames []string
	// LookPath reports whether an executable is on PATH.
	LookPath func(string) bool
	// Exists reports whether a file or folder exists.
	Exists func(string) bool
}

// Detect matches every catalog app against the inventory.
func Detect(cat *catalog.Catalog, inv Inventory) map[string]Installed {
	out := map[string]Installed{}
	for id, a := range cat.Apps {
		var src []string
		var version string
		if v, ok := inv.Winget[strings.ToLower(a.Winget)]; ok {
			src = append(src, "winget")
			version = v
		}
		for _, pattern := range a.Detect.Registry {
			re, err := regexp.Compile("(?i)" + pattern)
			if err != nil {
				continue
			}
			for _, n := range inv.DisplayNames {
				if re.MatchString(n) {
					src = append(src, "registry")
					goto registryDone
				}
			}
		}
	registryDone:
		if inv.LookPath != nil {
			for _, c := range a.Detect.Commands {
				if inv.LookPath(c) {
					src = append(src, "path")
					break
				}
			}
		}
		if inv.Exists != nil {
			for _, p := range a.Detect.Paths {
				if inv.Exists(expandWindowsVars(p)) {
					src = append(src, "file")
					break
				}
			}
		}
		if len(src) > 0 {
			out[id] = Installed{ID: id, Version: version, Sources: src}
		}
	}
	return out
}

var winVar = regexp.MustCompile(`%([^%]+)%`)

func expandWindowsVars(p string) string {
	return winVar.ReplaceAllStringFunc(p, func(m string) string {
		if v := os.Getenv(m[1 : len(m)-1]); v != "" {
			return v
		}
		return m
	})
}

// LookPath reports whether name resolves on PATH.
func LookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Exists reports whether a path exists.
func Exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
