package eol

import (
	"math"
	"sort"
	"time"
)

// SoonDays is how close an end date has to be for a finding to be "soon".
const SoonDays = 180

// StaleDays is the age of the data after which the report says it may be out of date (about 12
// months).
const StaleDays = 365

// Status is what a finding says about one installed instance.
type Status string

const (
	// Ended: the release line is past its last day of support.
	Ended Status = "ended"
	// Soon: it ends within SoonDays days (the last day itself included).
	Soon Status = "soon"
	// OK: supported, with an end date further away or none announced.
	OK Status = "ok"
	// Unknown: the product is known but the installed version is missing or matches no cycle.
	Unknown Status = "unknown"
)

// Installed is one installed catalog app and the version found for it ("" when the scan could not
// read one).
type Installed struct {
	AppID   string
	Version string
}

// Replacement is a supported release line the installed one could be replaced with.
type Replacement struct {
	Cycle string `json:"cycle"`
	End   string `json:"end,omitempty"`
	LTS   bool   `json:"lts,omitempty"`
}

// Finding is the result for one installed instance. All prose is built by the interface from
// these fields.
type Finding struct {
	AppID       string `json:"appId"`
	Product     string `json:"product"`
	ProductName string `json:"productName"`
	Version     string `json:"version"`
	// Cycle is the matched release line; empty for Unknown.
	Cycle  string `json:"cycle"`
	Status Status `json:"status"`
	// End is the last day of support (YYYY-MM-DD); empty when none is announced or unknown.
	End string `json:"end,omitempty"`
	// Days is End minus today: negative once it has ended. Absent when End is.
	Days *int `json:"days,omitempty"`
	LTS  bool `json:"lts,omitempty"`
	// Source is the page the date comes from.
	Source string `json:"source"`
	// Replace lists supported newer release lines (at most 3, long-term ones preferred).
	Replace []Replacement `json:"replace"`
	// Apps are catalog apps that install one of those lines, in the order to offer them.
	Apps []string `json:"apps"`
	// SelfUpdate is true when the installed app itself always installs the newest release, so
	// updating it is the way to move on.
	SelfUpdate bool `json:"selfUpdate,omitempty"`
}

// Report is what the page shows.
type Report struct {
	// AsOf is the day the bundled data was written.
	AsOf string `json:"asOf"`
	// AgeDays is how old the data is today (never negative).
	AgeDays int `json:"ageDays"`
	// Stale is true when the data is older than StaleDays.
	Stale    bool      `json:"stale"`
	Findings []Finding `json:"findings"`
}

// day is a calendar date with no time of day, so arithmetic on it counts whole days across month
// ends and leap days.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func daysBetween(from, to time.Time) int {
	return int(day(to).Sub(day(from)) / (24 * time.Hour))
}

// Build matches the installed apps against the data and returns the report as of now. The
// calendar day is the one in now's own location. A product can appear several times: every
// installed app gets its own finding.
func (d *Data) Build(installed []Installed, now time.Time) Report {
	byApp := map[string]*Product{}
	for i := range d.Products {
		for _, id := range d.Products[i].Apps {
			byApp[id] = &d.Products[i]
		}
	}
	r := Report{AsOf: d.AsOf, Findings: []Finding{}}
	if asOf, err := ParseDay(d.AsOf); err == nil {
		if n := daysBetween(asOf, now); n > 0 {
			r.AgeDays = n
		}
	}
	r.Stale = r.AgeDays > StaleDays
	for _, in := range installed {
		p := byApp[in.AppID]
		if p == nil {
			continue
		}
		r.Findings = append(r.Findings, p.finding(in, now))
	}
	sortFindings(r.Findings)
	return r
}

func (p *Product) finding(in Installed, now time.Time) Finding {
	f := Finding{
		AppID: in.AppID, Product: p.ID, ProductName: p.Name, Version: in.Version,
		Status: Unknown, Source: p.Source, Replace: []Replacement{}, Apps: []string{},
	}
	i := p.Match(in.Version)
	if i < 0 {
		return f
	}
	c := p.Cycles[i]
	f.Cycle, f.LTS = c.Cycle, c.LTS
	f.Status = OK
	if end, err := ParseDay(c.EOL); err == nil {
		n := daysBetween(now, end)
		f.End, f.Days = c.EOL, &n
		switch {
		case n < 0:
			f.Status = Ended
		case n <= SoonDays:
			f.Status = Soon
		}
	}
	if f.Status == Ended || f.Status == Soon {
		f.Replace, f.Apps, f.SelfUpdate = p.replacements(i, in.AppID, now)
	}
	return f
}

// supportedFor reports whether a cycle is still supported on now, and whether it stays so for a
// while (more than SoonDays).
func (c Cycle) supportedFor(now time.Time) (supported, lasting bool) {
	end, err := ParseDay(c.EOL)
	if err != nil {
		return true, true // no end announced
	}
	n := daysBetween(now, end)
	return n >= 0, n > SoonDays
}

// replacements picks, among the lines newer than the installed one (index from), the supported
// ones to move to: those that will last, else any still supported; long-term lines first when the
// product has them. It also says which catalog apps install them, never the app that is already
// installed.
func (p *Product) replacements(from int, self string, now time.Time) (rep []Replacement, apps []string, selfUpdate bool) {
	var lasting, supported []Cycle
	for _, c := range p.Cycles[:from] {
		ok, long := c.supportedFor(now)
		if ok {
			supported = append(supported, c)
		}
		if ok && long {
			lasting = append(lasting, c)
		}
	}
	pick := lasting
	if len(pick) == 0 {
		pick = supported
	}
	var lts []Cycle
	for _, c := range pick {
		if c.LTS {
			lts = append(lts, c)
		}
	}
	if len(lts) > 0 {
		pick = lts
	}
	if len(pick) > 3 {
		pick = pick[:3]
	}
	rep = make([]Replacement, 0, len(pick))
	apps = []string{}
	seen := map[string]bool{self: true}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			apps = append(apps, id)
		}
	}
	for _, c := range pick {
		rep = append(rep, Replacement{Cycle: c.Cycle, End: c.EOL, LTS: c.LTS})
		add(c.App)
	}
	if len(pick) > 0 {
		// Apps that follow a track come after the fixed lines: long-term ones first, then the
		// newest, each group by id so the order never depends on map iteration.
		follows := make([]string, 0, len(p.Follows))
		for id := range p.Follows {
			follows = append(follows, id)
		}
		sort.Strings(follows)
		if len(lts) > 0 {
			for _, id := range follows {
				if p.Follows[id] == TrackLTS {
					selfUpdate = selfUpdate || id == self // updating the installed app itself is the way
					add(id)
				}
			}
		}
		for _, id := range follows {
			if p.Follows[id] == TrackLatest {
				selfUpdate = selfUpdate || id == self
				add(id)
			}
		}
	}
	return rep, apps, selfUpdate
}

// daysKey sorts findings with no end date after the dated ones.
func daysKey(f Finding) int {
	if f.Days == nil {
		return math.MaxInt
	}
	return *f.Days
}

var statusRank = map[Status]int{Ended: 0, Soon: 1, Unknown: 2, OK: 3}

// sortFindings orders what needs attention first: ended (the longest-ended first), then soon
// (the nearest first), then unknown, then ok; ties by product name, version and app id so the
// order never depends on map iteration.
func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if ra, rb := statusRank[a.Status], statusRank[b.Status]; ra != rb {
			return ra < rb
		}
		if da, db := daysKey(a), daysKey(b); da != db {
			return da < db
		}
		if a.ProductName != b.ProductName {
			return a.ProductName < b.ProductName
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.AppID < b.AppID
	})
}
