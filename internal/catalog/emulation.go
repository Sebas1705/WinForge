package catalog

import (
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// System is a console, computer or engine that emulators play.
type System struct {
	ID string `yaml:"id" json:"id"`
	EN string `yaml:"en" json:"en"`
	ES string `yaml:"es" json:"es"`
}

// Source is a link to where free games, homebrew or patches for a system live.
// WinForge only links to it: nothing is downloaded or hosted.
type Source struct {
	ID     string `yaml:"id" json:"id"`
	System string `yaml:"system" json:"system"` // a System id, or "any"
	Kind   string `yaml:"kind" json:"kind"`
	Name   string `yaml:"name" json:"name"`
	EN     string `yaml:"en" json:"en"`
	ES     string `yaml:"es" json:"es"`
	URL    string `yaml:"url" json:"url"`
	// Base means the download is a patch or tool that needs the person's own
	// copy of the original game.
	Base bool `yaml:"base,omitempty" json:"base,omitempty"`
}

// Emulation is what the "games for your emulators" page shows.
type Emulation struct {
	Systems []System `yaml:"systems" json:"systems"`
	// Emulators maps a catalog app id to the systems it plays.
	Emulators map[string][]string `yaml:"emulators" json:"emulators"`
	Sources   []Source            `yaml:"sources" json:"sources"`
}

// SourceKinds are the kinds a source may have.
var SourceKinds = []string{"homebrew", "freegames", "hacks", "translations", "randomizers", "tools", "community"}

// blockedHosts are sites that distribute commercial games without permission.
// They must never be linked, whatever else they offer.
var blockedHosts = []string{
	"emuparadise", "romsmania", "coolrom", "romspure", "romsgames", "romsfun", "vimm.net", "cdromance", "nopaystation",
	"edgeemu", "romulation", "theisozone", "romsmode", "freeroms", "loveroms", "wowroms", "retrostic", "romhustler",
}

// loadEmulation reads emulation.yml (optional) and validates it against the apps.
func (c *Catalog) loadEmulation(fsys fs.FS) error {
	b, err := fs.ReadFile(fsys, "emulation.yml")
	if err != nil {
		return nil // optional
	}
	var e Emulation
	if err := yaml.UnmarshalWithOptions(b, &e, yaml.Strict()); err != nil {
		return fmt.Errorf("emulation.yml: %w", err)
	}
	if err := e.validate(c); err != nil {
		return fmt.Errorf("emulation.yml: %w", err)
	}
	c.Emulation = &e
	return nil
}

func (e *Emulation) validate(c *Catalog) error {
	systems := map[string]bool{}
	for _, s := range e.Systems {
		switch {
		case !validID(s.ID):
			return fmt.Errorf("system %q: id must be lowercase letters, digits and dashes", s.ID)
		case systems[s.ID]:
			return fmt.Errorf("system %q listed twice", s.ID)
		case s.EN == "" || s.ES == "":
			return fmt.Errorf("system %q needs en and es", s.ID)
		}
		systems[s.ID] = true
	}
	ids := make([]string, 0, len(e.Emulators))
	for id := range e.Emulators {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if c.Apps[id] == nil {
			return fmt.Errorf("emulator %q is not a catalog app", id)
		}
		if len(e.Emulators[id]) == 0 {
			return fmt.Errorf("emulator %q plays no system", id)
		}
		for _, s := range e.Emulators[id] {
			if !systems[s] {
				return fmt.Errorf("emulator %q: unknown system %q", id, s)
			}
		}
	}
	kinds := map[string]bool{}
	for _, k := range SourceKinds {
		kinds[k] = true
	}
	seen := map[string]bool{}
	for _, s := range e.Sources {
		switch {
		case !validID(s.ID):
			return fmt.Errorf("source %q: id must be lowercase letters, digits and dashes", s.ID)
		case seen[s.ID]:
			return fmt.Errorf("source %q listed twice", s.ID)
		case s.System != "any" && !systems[s.System]:
			return fmt.Errorf("source %q: unknown system %q", s.ID, s.System)
		case !kinds[s.Kind]:
			return fmt.Errorf("source %q: unknown kind %q", s.ID, s.Kind)
		case s.Name == "" || s.EN == "" || s.ES == "":
			return fmt.Errorf("source %q needs name, en and es", s.ID)
		case len([]rune(s.EN)) > 140 || len([]rune(s.ES)) > 160:
			return fmt.Errorf("source %q: description is too long for a card", s.ID)
		}
		u, err := url.Parse(s.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("source %q: url must be https", s.ID)
		}
		host := strings.ToLower(u.Host)
		for _, bad := range blockedHosts {
			if strings.Contains(host, bad) {
				return fmt.Errorf("source %q links to %s, which distributes games without permission", s.ID, u.Host)
			}
		}
		seen[s.ID] = true
	}
	return nil
}
