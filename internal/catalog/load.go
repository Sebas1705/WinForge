package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Catalog is the loaded, validated catalog.
type Catalog struct {
	Apps     map[string]*App
	Profiles map[string]*Profile
	Recipes  map[string]*Recipe
	// Featured lists the popular apps in display order.
	Featured []string
	// Emulation says which systems each emulator plays and where free games live.
	Emulation *Emulation
	// Games are the games that can be installed from WinForge.
	Games []Game
}

// Load reads apps/*.yml, profiles/*.yml and recipes/*.yml from fsys and
// validates cross references. Unknown YAML fields are errors, so a typo in
// the catalog cannot silently drop a setting.
func Load(fsys fs.FS) (*Catalog, error) {
	c := &Catalog{Apps: map[string]*App{}, Profiles: map[string]*Profile{}, Recipes: map[string]*Recipe{}}

	var apps []*App
	if err := readAll(fsys, "apps", &apps); err != nil {
		return nil, err
	}
	for _, a := range apps {
		if _, dup := c.Apps[a.ID]; dup {
			return nil, fmt.Errorf("duplicate app id %q", a.ID)
		}
		if a.Trust == "" {
			a.Trust = TrustVendor
		}
		c.Apps[a.ID] = a
	}
	var profiles []*Profile
	if err := readAll(fsys, "profiles", &profiles); err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if _, dup := c.Profiles[p.ID]; dup {
			return nil, fmt.Errorf("duplicate profile id %q", p.ID)
		}
		c.Profiles[p.ID] = p
	}
	var recipes []*Recipe
	if err := readAll(fsys, "recipes", &recipes); err != nil {
		return nil, err
	}
	for _, r := range recipes {
		if _, dup := c.Recipes[r.ID]; dup {
			return nil, fmt.Errorf("duplicate recipe id %q", r.ID)
		}
		c.Recipes[r.ID] = r
	}
	var overlays []*DetectOverlay
	if err := readAll(fsys, "detect", &overlays); err != nil {
		return nil, err
	}
	for _, o := range overlays {
		a := c.Apps[o.ID]
		if a == nil {
			return nil, fmt.Errorf("detect overlay for unknown app %q", o.ID)
		}
		a.Detect.Registry = union(a.Detect.Registry, o.Registry)
	}
	for _, a := range c.Apps {
		a.OpenSource = IsOpenSource(a.License)
	}
	if err := c.loadFeatured(fsys); err != nil {
		return nil, err
	}
	if err := c.loadTaglines(fsys); err != nil {
		return nil, err
	}
	if err := c.loadEmulation(fsys); err != nil {
		return nil, err
	}
	if err := c.loadGames(fsys); err != nil {
		return nil, err
	}
	if errs := c.Validate(); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("catalog invalid:\n  %s", strings.Join(msgs, "\n  "))
	}
	return c, nil
}

// readAll decodes every .yml in dir; each file is a YAML list of T.
func readAll[T any](fsys fs.FS, dir string, out *[]T) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".yml" {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		var items []T
		if err := yaml.UnmarshalWithOptions(b, &items, yaml.Strict()); err != nil {
			return fmt.Errorf("%s/%s: %w", dir, e.Name(), err)
		}
		*out = append(*out, items...)
	}
	return nil
}

// Validate checks structural rules and cross references.
func (c *Catalog) Validate() []error {
	var errs []error
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }

	for _, id := range sortedKeys(c.Apps) {
		a := c.Apps[id]
		switch {
		case !validID(a.ID):
			add("app %q: id must be lowercase letters, digits and dashes", a.ID)
		case a.Name == "" || a.Category == "" || a.Description == "":
			add("app %q: name, category and description are required", a.ID)
		case !strings.HasPrefix(a.Homepage, "https://"):
			add("app %q: homepage must be an https URL", a.ID)
		case a.Winget == "" || a.Publisher == "":
			add("app %q: winget and publisher are required", a.ID)
		case a.Scope != "" && a.Scope != "user" && a.Scope != "machine":
			add("app %q: scope must be user or machine", a.ID)
		}
		for _, pattern := range a.Detect.Registry {
			if _, err := regexp.Compile("(?i)" + pattern); err != nil {
				add("app %q: bad detect.registry pattern %q: %v", a.ID, pattern, err)
			}
		}
		for _, r := range a.Requires {
			if c.Apps[r] == nil {
				add("app %q: requires unknown app %q", a.ID, r)
			}
		}
		for _, r := range a.Recipes {
			if c.Recipes[r] == nil {
				add("app %q: unknown recipe %q", a.ID, r)
			}
		}
	}
	for _, id := range sortedKeys(c.Recipes) {
		r := c.Recipes[id]
		if r.Description == "" || strings.TrimSpace(r.PowerShell) == "" {
			add("recipe %q: description and powershell are required", r.ID)
		}
		for _, a := range r.After {
			if c.Apps[a] == nil {
				add("recipe %q: after unknown app %q", r.ID, a)
			}
		}
	}
	for _, id := range sortedKeys(c.Profiles) {
		p := c.Profiles[id]
		if !validID(p.ID) || p.Name == "" {
			add("profile %q: valid id and name are required", p.ID)
		}
		if _, err := c.Resolve(p.ID); err != nil {
			add("profile %q: %v", p.ID, err)
		}
	}
	// Two catalog entries pointing at the same winget package are a mistake.
	seen := map[string]string{}
	for _, id := range sortedKeys(c.Apps) {
		w := strings.ToLower(c.Apps[id].Winget)
		if prev, ok := seen[w]; ok {
			add("apps %q and %q share winget id %q", prev, id, c.Apps[id].Winget)
		}
		seen[w] = id
	}
	return errs
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func validID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// loadFeatured reads featured.yml (optional) and attaches taglines. Every id
// must exist and every line needs both languages, kept short enough for a card.
func (c *Catalog) loadFeatured(fsys fs.FS) error {
	b, err := fs.ReadFile(fsys, "featured.yml")
	if err != nil {
		return nil // optional
	}
	var entries []FeaturedEntry
	if err := yaml.UnmarshalWithOptions(b, &entries, yaml.Strict()); err != nil {
		return fmt.Errorf("featured.yml: %w", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		a := c.Apps[e.ID]
		switch {
		case a == nil:
			return fmt.Errorf("featured.yml: unknown app %q", e.ID)
		case seen[e.ID]:
			return fmt.Errorf("featured.yml: %q listed twice", e.ID)
		case e.EN == "" || e.ES == "":
			return fmt.Errorf("featured.yml: %q needs en and es", e.ID)
		case len([]rune(e.EN)) > 60 || len([]rune(e.ES)) > 60:
			return fmt.Errorf("featured.yml: %q tagline is longer than 60 characters", e.ID)
		}
		seen[e.ID] = true
		a.Tagline = &Tagline{EN: e.EN, ES: e.ES}
		c.Featured = append(c.Featured, e.ID)
	}
	return nil
}

// loadTaglines reads taglines.yml (optional): one-line descriptions like the
// featured ones, for apps that should read well on a card but are not part of
// the "popular" list. An app may have a tagline from one file only.
func (c *Catalog) loadTaglines(fsys fs.FS) error {
	b, err := fs.ReadFile(fsys, "taglines.yml")
	if err != nil {
		return nil // optional
	}
	var entries []FeaturedEntry
	if err := yaml.UnmarshalWithOptions(b, &entries, yaml.Strict()); err != nil {
		return fmt.Errorf("taglines.yml: %w", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		a := c.Apps[e.ID]
		switch {
		case a == nil:
			return fmt.Errorf("taglines.yml: unknown app %q", e.ID)
		case seen[e.ID]:
			return fmt.Errorf("taglines.yml: %q listed twice", e.ID)
		case a.Tagline != nil:
			return fmt.Errorf("taglines.yml: %q already has a tagline in featured.yml", e.ID)
		case e.EN == "" || e.ES == "":
			return fmt.Errorf("taglines.yml: %q needs en and es", e.ID)
		case len([]rune(e.EN)) > 60 || len([]rune(e.ES)) > 60:
			return fmt.Errorf("taglines.yml: %q tagline is longer than 60 characters", e.ID)
		}
		seen[e.ID] = true
		a.Tagline = &Tagline{EN: e.EN, ES: e.ES}
	}
	return nil
}
