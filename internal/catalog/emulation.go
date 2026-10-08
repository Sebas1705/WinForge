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
	// Run says how to start an emulator on a game file, for the ones whose
	// command line is known. Emulators without an entry are not launched.
	Run map[string]RunSpec `yaml:"run,omitempty" json:"run"`
}

// RunSpec is how to find and start an emulator.
type RunSpec struct {
	// Exes are the program's file names, in order of preference.
	Exes []string `yaml:"exes" json:"exes"`
	// Hints are lowercase words a folder name has when the emulator lives in it
	// ("mgba" for C:\Tools\mGBA).
	Hints []string `yaml:"hints" json:"hints"`
	// Args start a game: "{file}" is replaced by the game's path.
	Args []string `yaml:"args,omitempty" json:"args,omitempty"`
	// Register, when set, is run once after a game is installed so the emulator
	// knows about it ("{dir}" is the game's folder). ScummVM finds games this way.
	Register []string `yaml:"register,omitempty" json:"register,omitempty"`
}

func sortedRunIDs(m map[string]RunSpec) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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
	for _, id := range sortedRunIDs(e.Run) {
		r := e.Run[id]
		if _, ok := e.Emulators[id]; !ok {
			return fmt.Errorf("run: %q is not an emulator listed above", id)
		}
		if len(r.Exes) == 0 || len(r.Hints) == 0 {
			return fmt.Errorf("run: %q needs exes and hints", id)
		}
		for _, x := range r.Exes {
			if x == "" || strings.ContainsAny(x, `/\:`) || !strings.HasSuffix(strings.ToLower(x), ".exe") {
				return fmt.Errorf("run: %q: %q must be a plain .exe name", id, x)
			}
		}
		hasFile := false
		for _, a := range r.Args {
			hasFile = hasFile || strings.Contains(a, "{file}")
		}
		if !hasFile && len(r.Register) == 0 {
			return fmt.Errorf("run: %q has neither args with {file} nor register", id)
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
