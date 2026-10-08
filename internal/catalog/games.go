package catalog

import (
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

// Game is a game that may be installed straight from WinForge. Only games whose
// authors allow free redistribution are listed (freeware, open source, public
// domain), each with the SHA-256 of the exact file, so what is downloaded is
// known before it is run.
type Game struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name" json:"name"`
	System  string `yaml:"system" json:"system"`
	EN      string `yaml:"en" json:"en"`
	ES      string `yaml:"es" json:"es"`
	License string `yaml:"license" json:"license"`
	// Homepage is where the permission to share it is stated.
	Homepage string `yaml:"homepage" json:"homepage"`
	URL      string `yaml:"url" json:"url"`
	SHA256   string `yaml:"sha256" json:"sha256"`
	Size     int64  `yaml:"size" json:"size"`
	// Kind is "zip" (extracted into the game's folder) or "file" (saved as File).
	Kind string `yaml:"kind" json:"kind"`
	File string `yaml:"file,omitempty" json:"file,omitempty"`
	// Entry is the file inside the game's folder that an emulator is given to
	// play it. Empty for games a front end finds by itself (ScummVM).
	Entry string `yaml:"entry,omitempty" json:"entry,omitempty"`
}

// gameHosts are the only hosts games may be downloaded from. GitHub redirects a
// release to its own CDN, which the downloader follows over https only.
var gameHosts = []string{"downloads.scummvm.org", "github.com"}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// MaxGameSize bounds one game's download.
const MaxGameSize = int64(2) << 30

type gamesFile struct {
	Games []Game `yaml:"games"`
}

// loadGames reads games.yml (optional) and validates it against the systems.
func (c *Catalog) loadGames(fsys fs.FS) error {
	b, err := fs.ReadFile(fsys, "games.yml")
	if err != nil {
		return nil // optional
	}
	var f gamesFile
	if err := yaml.UnmarshalWithOptions(b, &f, yaml.Strict()); err != nil {
		return fmt.Errorf("games.yml: %w", err)
	}
	systems := map[string]bool{}
	if c.Emulation != nil {
		for _, s := range c.Emulation.Systems {
			systems[s.ID] = true
		}
	}
	seen := map[string]bool{}
	for _, g := range f.Games {
		if err := g.validate(systems); err != nil {
			return fmt.Errorf("games.yml: game %q: %w", g.ID, err)
		}
		if seen[g.ID] {
			return fmt.Errorf("games.yml: game %q listed twice", g.ID)
		}
		seen[g.ID] = true
	}
	c.Games = f.Games
	return nil
}

func (g Game) validate(systems map[string]bool) error {
	switch {
	case !validID(g.ID):
		return fmt.Errorf("id must be lowercase letters, digits and dashes")
	case g.Name == "" || g.EN == "" || g.ES == "" || g.License == "":
		return fmt.Errorf("name, en, es and license are required")
	case len([]rune(g.EN)) > 140 || len([]rune(g.ES)) > 160:
		return fmt.Errorf("description is too long for a card")
	case !systems[g.System]:
		return fmt.Errorf("unknown system %q", g.System)
	case !sha256Hex.MatchString(g.SHA256):
		return fmt.Errorf("sha256 must be 64 lowercase hex digits")
	case g.Size <= 0 || g.Size > MaxGameSize:
		return fmt.Errorf("size must be between 1 byte and 2 GB")
	case g.Kind != "zip" && g.Kind != "file":
		return fmt.Errorf("kind must be zip or file")
	case g.Kind == "file" && (g.File == "" || strings.ContainsAny(g.File, `/\:`) || g.File == "." || g.File == ".."):
		return fmt.Errorf("a file game needs a plain file name")
	case g.Entry != "" && (!cleanRelPath(g.Entry)):
		return fmt.Errorf("entry must be a relative path inside the game")
	case g.Kind == "file" && g.Entry != "" && g.Entry != g.File:
		return fmt.Errorf("a file game's entry is its file")
	}
	hp, err := url.Parse(g.Homepage)
	if err != nil || hp.Scheme != "https" || hp.Host == "" {
		return fmt.Errorf("homepage must be https")
	}
	u, err := url.Parse(g.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("url must be https")
	}
	ok := false
	for _, h := range gameHosts {
		ok = ok || strings.EqualFold(u.Host, h)
	}
	if !ok {
		return fmt.Errorf("downloads are only allowed from %s, not %s", strings.Join(gameHosts, " or "), u.Host)
	}
	return nil
}

func cleanRelPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, `\:`) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}
