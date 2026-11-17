// Package eol answers one question about the software installed on this PC: has the release line
// it belongs to reached the end of its support, or will it soon?
//
// Privacy is the design constraint. The answer comes from a table that ships inside WinForge
// (catalogdata/eol.yml, written by cmd/winforge-eol and reviewed by a person before it is
// committed). Nothing about the installed software or its versions ever leaves the PC, and nothing
// is downloaded at run time. The cost is that the table is as old as the WinForge version, which is
// why every report carries the date the table was written (AsOf) and says when it is stale.
//
// The package is pure: no file system, no clock, no network. Callers pass the installed apps and
// the current time.
package eol

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// File is the name of the data file inside catalogdata.
const File = "eol.yml"

// Scheme names how a product's version strings map to its release cycles.
const (
	// SchemeDotted is the default: a cycle such as "3.13" matches versions "3.13", "3.13.5" and
	// "3.13.0rc1", compared one dotted component at a time.
	SchemeDotted = "dotted"
	// SchemeJava is SchemeDotted plus the legacy "1.8.0_402" spelling of Java 8.
	SchemeJava = "java"
)

// Track says which release an always-newest app installs.
const (
	TrackLatest = "latest"
	TrackLTS    = "lts"
)

// Data is the whole eol.yml.
type Data struct {
	// AsOf is the day (YYYY-MM-DD) the table was checked against its sources.
	AsOf     string    `yaml:"asOf" json:"asOf"`
	Products []Product `yaml:"products" json:"products"`
}

// Product is one piece of software with release cycles that end.
type Product struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	// Source is the page the dates come from (https).
	Source string `yaml:"source" json:"source"`
	Scheme string `yaml:"scheme,omitempty" json:"scheme,omitempty"`
	// Apps are the catalog app ids that are an installed instance of this product.
	Apps []string `yaml:"apps" json:"apps"`
	// Follows maps a catalog app that always installs the newest release (or the newest LTS) to
	// that track, so it can be offered as the way to move on without naming a fixed cycle.
	Follows map[string]string `yaml:"follows,omitempty" json:"follows,omitempty"`
	// Cycles lists the release lines, newest first.
	Cycles []Cycle `yaml:"cycles" json:"cycles"`
}

// Cycle is one release line of a product.
type Cycle struct {
	// Cycle is the version prefix that belongs to it: "3.13", "21", "17.14".
	Cycle string `yaml:"cycle" json:"cycle"`
	// EOL is the last day of support (YYYY-MM-DD). Empty means no end has been announced.
	EOL string `yaml:"eol,omitempty" json:"eol,omitempty"`
	// LTS marks a long-term-support line.
	LTS bool `yaml:"lts,omitempty" json:"lts,omitempty"`
	// App is the catalog app that installs this cycle, when there is one.
	App string `yaml:"app,omitempty" json:"app,omitempty"`
}

// Parse reads and decodes an eol.yml. Unknown fields are errors so a typo cannot drop a date.
func Parse(b []byte) (*Data, error) {
	var d Data
	if err := yaml.UnmarshalWithOptions(b, &d, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	return &d, nil
}

// Load reads eol.yml from fsys (the embedded catalogdata) and decodes it.
func Load(fsys fs.FS) (*Data, error) {
	b, err := fs.ReadFile(fsys, File)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

var (
	idRe    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	cycleRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

// DayLayout is the date format used in the data and in findings.
const DayLayout = "2006-01-02"

// ParseDay reads a YYYY-MM-DD date.
func ParseDay(s string) (time.Time, error) { return time.Parse(DayLayout, s) }

// Validate checks the data's structure. appExists says whether a catalog app id exists; it is a
// function so this package does not depend on the catalog. All problems are returned, not just
// the first.
func (d *Data) Validate(appExists func(id string) bool) []error {
	var errs []error
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }

	if _, err := ParseDay(d.AsOf); err != nil {
		add("asOf %q is not a YYYY-MM-DD date", d.AsOf)
	}
	if len(d.Products) == 0 {
		add("no products")
	}
	seenProduct := map[string]bool{}
	appOwner := map[string]string{}
	for _, p := range d.Products {
		switch {
		case !idRe.MatchString(p.ID):
			add("product %q: id must be lowercase letters, digits and dashes", p.ID)
		case seenProduct[p.ID]:
			add("product %q listed twice", p.ID)
		}
		seenProduct[p.ID] = true
		if strings.TrimSpace(p.Name) == "" {
			add("product %q: name is required", p.ID)
		}
		if !strings.HasPrefix(p.Source, "https://") {
			add("product %q: source must be an https URL", p.ID)
		}
		if p.Scheme != "" && p.Scheme != SchemeDotted && p.Scheme != SchemeJava {
			add("product %q: unknown scheme %q", p.ID, p.Scheme)
		}
		if len(p.Apps) == 0 {
			add("product %q: apps is empty, so nothing could match it", p.ID)
		}
		for _, id := range p.Apps {
			if !appExists(id) {
				add("product %q: %q is not a catalog app", p.ID, id)
			}
			if other, dup := appOwner[id]; dup {
				add("app %q belongs to products %q and %q", id, other, p.ID)
			}
			appOwner[id] = p.ID
		}
		for id, track := range p.Follows {
			if !appExists(id) {
				add("product %q: follows %q, which is not a catalog app", p.ID, id)
			}
			if track != TrackLatest && track != TrackLTS {
				add("product %q: follows %q with unknown track %q", p.ID, id, track)
			}
		}
		if len(p.Cycles) == 0 {
			add("product %q: no cycles", p.ID)
		}
		seen := map[string]bool{}
		for i, c := range p.Cycles {
			switch {
			case !cycleRe.MatchString(c.Cycle):
				add("product %q: cycle %q must be dotted numbers", p.ID, c.Cycle)
			case seen[c.Cycle]:
				add("product %q: cycle %q listed twice", p.ID, c.Cycle)
			case i > 0 && cycleRe.MatchString(p.Cycles[i-1].Cycle) && CompareCycles(c.Cycle, p.Cycles[i-1].Cycle) >= 0:
				add("product %q: cycle %q must come after %q (newest first)", p.ID, c.Cycle, p.Cycles[i-1].Cycle)
			}
			seen[c.Cycle] = true
			if c.EOL != "" {
				if _, err := ParseDay(c.EOL); err != nil {
					add("product %q cycle %q: eol %q is not a YYYY-MM-DD date", p.ID, c.Cycle, c.EOL)
				}
			}
			if c.App != "" && !appExists(c.App) {
				add("product %q cycle %q: app %q is not a catalog app", p.ID, c.Cycle, c.App)
			}
		}
	}
	return errs
}

// CompareCycles compares two dotted-number cycles numerically, component by component: "3.10" is
// newer than "3.9", and "17" newer than "9.6". The generator uses it to order cycles newest first.
func CompareCycles(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = trimZeros(as[i])
		}
		if i < len(bs) {
			y = trimZeros(bs[i])
		}
		if c := compareNumbers(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func trimZeros(s string) string {
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

// compareNumbers compares two digit strings without zeros in front; a missing one is smaller.
func compareNumbers(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return -1
	case b == "":
		return 1
	case len(a) != len(b):
		if len(a) < len(b) {
			return -1
		}
		return 1
	case a < b:
		return -1
	default:
		return 1
	}
}
