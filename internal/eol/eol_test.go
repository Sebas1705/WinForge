package eol

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func prod(scheme string, cycles ...string) *Product {
	p := &Product{ID: "p", Name: "P", Scheme: scheme}
	for _, c := range cycles {
		p.Cycles = append(p.Cycles, Cycle{Cycle: c})
	}
	return p
}

func TestMatchTable(t *testing.T) {
	py := prod("", "3.14", "3.13", "3.10", "3.1", "3.9")
	jv := prod(SchemeJava, "25", "21", "17", "11", "8")
	dn := prod("", "10", "9", "8")
	pg := prod("", "17", "16", "9.6")
	cases := []struct {
		name string
		p    *Product
		v    string
		want string // matched cycle, "" for none
	}{
		{"python exact two parts", py, "3.13", "3.13"},
		{"python patch", py, "3.13.5", "3.13"},
		{"3.10 is not 3.1", py, "3.10.4", "3.10"},
		{"3.1 is not 3.10", py, "3.1.5", "3.1"},
		{"3.1 does not match 3.12 (no such cycle)", py, "3.12.1", ""},
		{"3.9 is not 3.90", py, "3.90.1", ""},
		{"too short for the cycle", py, "3", ""},
		{"pre-release glued to the last number", py, "3.13.0rc1", "3.13"},
		{"pre-release with dash", py, "3.14.0-rc.2", "3.14"},
		{"leading zero", py, "3.09.1", "3.9"},
		{"v prefix", dn, "v10.0.100", "10"},
		{"capital V prefix", dn, "V9.0.2", "9"},
		{"four parts", dn, "8.0.404.1", "8"},
		{"build suffix with plus", jv, "21.0.4+7", "21"},
		{"build suffix with dash", pg, "17.6-1", "17"},
		{"trailing text after a space", py, "3.13.5 (64-bit)", "3.13"},
		{"major only matches a major cycle", jv, "17", "17"},
		{"17.0.9 is not 1.17", jv, "17.0.9", "17"},
		{"1.17 is not 17 (no such Java)", jv, "1.17.0", ""},
		{"1.17 against a plain product", dn, "1.17", ""},
		{"java 1.8 spelling", jv, "1.8.0_402", "8"},
		{"java 8 modern spelling", jv, "8.0.432.6", "8"},
		{"java 1.8 needs the java scheme", prod("", "8", "1.8"), "1.8.0_402", "1.8"},
		{"21 is not 2", prod("", "2"), "21.0.1", ""},
		{"2 is not 21", prod("", "21"), "2.1", ""},
		{"postgres 9.6", pg, "9.6.24", "9.6"},
		{"postgres 9.5 is no cycle", pg, "9.5.25", ""},
		{"longest cycle wins", prod("", "3", "3.1"), "3.1.4", "3.1"},
		{"shorter cycle still matches", prod("", "3", "3.1"), "3.2.4", "3"},
		{"empty", py, "", ""},
		{"blank", py, "   ", ""},
		{"only text", py, "latest", ""},
		{"empty component", py, "3..13", ""},
		{"component with letters is not a number", py, "3.1x.5", ""},
		{"dash only", py, "-", ""},
	}
	for _, c := range cases {
		i := c.p.Match(c.v)
		got := ""
		if i >= 0 {
			got = c.p.Cycles[i].Cycle
		}
		if got != c.want {
			t.Errorf("%s: Match(%q) = %q, want %q", c.name, c.v, got, c.want)
		}
	}
}

func TestCompareCycles(t *testing.T) {
	order := []string{"26.10", "9.7", "3.10", "3.9", "3.1", "3", "2.7", "1.0"}
	for i := 0; i+1 < len(order); i++ {
		if CompareCycles(order[i], order[i+1]) <= 0 || CompareCycles(order[i+1], order[i]) >= 0 {
			t.Errorf("%s should be newer than %s", order[i], order[i+1])
		}
	}
	if CompareCycles("3.10", "3.10") != 0 || CompareCycles("3.010", "3.10") != 0 {
		t.Error("equal cycles compare unequal")
	}
}

// at builds a clock time. The zone is varied by tests that care about the calendar day.
func at(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

func data() *Data {
	return &Data{
		AsOf: "2026-11-17",
		Products: []Product{
			{
				ID: "python", Name: "Python", Source: "https://endoflife.date/python",
				Apps: []string{"py-310", "py-311", "py-313", "py-other"},
				Cycles: []Cycle{
					{Cycle: "3.15", EOL: "2031-10-31", App: "py-315"},
					{Cycle: "3.14", EOL: "2030-10-31", App: "py-314"},
					{Cycle: "3.13", EOL: "2029-10-31", App: "py-313"},
					{Cycle: "3.11", EOL: "2027-10-31"},
					{Cycle: "3.10", EOL: "2026-10-01"},
				},
			},
			{
				ID: "temurin", Name: "Temurin", Source: "https://endoflife.date/eclipse-temurin", Scheme: SchemeJava,
				Apps: []string{"jdk-8", "jdk-11", "jdk-17", "jdk-21", "jre-17"},
				Cycles: []Cycle{
					{Cycle: "25", EOL: "2031-09-30", LTS: true, App: "jdk-25"},
					{Cycle: "21", EOL: "2029-12-31", LTS: true, App: "jdk-21"},
					{Cycle: "17", EOL: "2027-10-31", LTS: true, App: "jdk-17"},
					{Cycle: "11", EOL: "2027-05-10", LTS: true, App: "jdk-11"},
					{Cycle: "8", EOL: "2030-12-31", LTS: true, App: "jdk-8"},
				},
			},
			{
				ID: "node", Name: "Node.js", Source: "https://endoflife.date/nodejs",
				Apps:    []string{"node-lts", "node-cur", "node-20"},
				Follows: map[string]string{"node-lts": TrackLTS, "node-cur": TrackLatest},
				Cycles: []Cycle{
					{Cycle: "26", EOL: "2029-04-30", LTS: true},
					{Cycle: "25", EOL: "2026-06-01"},
					{Cycle: "24", EOL: "2028-04-30", LTS: true},
					{Cycle: "22", EOL: "2027-04-30", LTS: true},
					{Cycle: "20", EOL: "2026-04-30", LTS: true, App: "node-20"},
				},
			},
			{
				ID: "go", Name: "Go", Source: "https://endoflife.date/go",
				Apps:    []string{"go"},
				Follows: map[string]string{"go": TrackLatest},
				Cycles:  []Cycle{{Cycle: "1.27"}, {Cycle: "1.26"}, {Cycle: "1.25", EOL: "2026-08-19"}},
			},
		},
	}
}

func byApp(r Report) map[string]Finding {
	m := map[string]Finding{}
	for _, f := range r.Findings {
		m[f.AppID] = f
	}
	return m
}

func TestDateLogicWithInjectedClock(t *testing.T) {
	d := &Data{AsOf: "2026-01-01", Products: []Product{{
		ID: "x", Name: "X", Apps: []string{"x1"}, Cycles: []Cycle{{Cycle: "1", EOL: "2026-06-30"}},
	}}}
	status := func(now time.Time) (Status, int) {
		f := d.Build([]Installed{{AppID: "x1", Version: "1.2.3"}}, now).Findings[0]
		return f.Status, *f.Days
	}
	check := func(now time.Time, want Status, days int) {
		t.Helper()
		s, n := status(now)
		if s != want || n != days {
			t.Errorf("at %s: %s %d days, want %s %d", now.Format(time.RFC3339), s, n, want, days)
		}
	}
	check(time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), OK, 181)  // 181 days out
	check(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Soon, 180)     // exactly 180 days: soon
	check(time.Date(2026, 1, 1, 23, 59, 59, 0, time.UTC), Soon, 180)  // the same day, late
	check(time.Date(2026, 6, 29, 23, 59, 59, 0, time.UTC), Soon, 1)   // the day before
	check(time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), Soon, 0)      // the last day is still supported
	check(time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC), Soon, 0)   // until the day is over
	check(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Ended, -1)     // the day after
	check(time.Date(2027, 6, 30, 12, 0, 0, 0, time.UTC), Ended, -365) // a year later
	// The calendar day is the one in the clock's own zone.
	tokyo := time.FixedZone("JST", 9*3600)
	check(time.Date(2026, 6, 30, 23, 0, 0, 0, tokyo), Soon, 0)
	check(time.Date(2026, 7, 1, 0, 30, 0, 0, tokyo), Ended, -1)
}

func TestDateLogicAcrossLeapDays(t *testing.T) {
	// 2028 is a leap year: 2028-02-29 exists and the arithmetic must count it.
	d := &Data{AsOf: "2028-01-01", Products: []Product{{
		ID: "x", Name: "X", Apps: []string{"x1"}, Cycles: []Cycle{{Cycle: "1", EOL: "2028-02-29"}, {Cycle: "0", EOL: "2028-03-01"}},
	}}}
	f := func(v string, now time.Time) Finding {
		return d.Build([]Installed{{AppID: "x1", Version: v}}, now).Findings[0]
	}
	if g := f("1.0", time.Date(2028, 2, 29, 9, 0, 0, 0, time.UTC)); g.Status != Soon || *g.Days != 0 {
		t.Errorf("last day on the leap day: %+v", g)
	}
	if g := f("1.0", time.Date(2028, 3, 1, 9, 0, 0, 0, time.UTC)); g.Status != Ended || *g.Days != -1 {
		t.Errorf("day after the leap day: %+v", g)
	}
	if g := f("0.1", time.Date(2028, 2, 28, 9, 0, 0, 0, time.UTC)); *g.Days != 2 {
		t.Errorf("2028-02-28 to 2028-03-01 is 2 days with a leap day: %+v", g)
	}
	if g := f("1.0", time.Date(2027, 2, 28, 9, 0, 0, 0, time.UTC)); *g.Days != 366 || g.Status != OK {
		t.Errorf("a year before a leap day: %+v", g)
	}
	// 180 days before 2028-02-29 is 2027-09-02; a day earlier it is 181 and still ok.
	if g := f("1.0", time.Date(2027, 9, 2, 9, 0, 0, 0, time.UTC)); g.Status != Soon || *g.Days != 180 {
		t.Errorf("%+v", g)
	}
	if g := f("1.0", time.Date(2027, 9, 1, 9, 0, 0, 0, time.UTC)); g.Status != OK || *g.Days != 181 {
		t.Errorf("%+v", g)
	}
}

func TestFindingsForInstalledApps(t *testing.T) {
	now := at(2026, 11, 17)
	r := data().Build([]Installed{
		{AppID: "py-310", Version: "3.10.11"},
		{AppID: "py-313", Version: "3.13.9"},
		{AppID: "py-311", Version: "3.11.4"},
		{AppID: "jdk-17", Version: "17.0.9+9"},
		{AppID: "jdk-8", Version: "1.8.0_402"},
		{AppID: "jdk-11", Version: "11.0.20"},
		{AppID: "go", Version: "1.25.3"},
		{AppID: "py-other", Version: ""},
		{AppID: "not-tracked", Version: "1.0"},
	}, now)
	f := byApp(r)
	if len(r.Findings) != 8 {
		t.Fatalf("an app the data does not know must be left out: %d findings", len(r.Findings))
	}
	if g := f["py-310"]; g.Status != Ended || g.End != "2026-10-01" || *g.Days != -47 || g.Cycle != "3.10" || g.ProductName != "Python" {
		t.Errorf("python 3.10: %+v", g)
	}
	if g := f["py-313"]; g.Status != OK || g.End != "2029-10-31" || len(g.Replace) != 0 {
		t.Errorf("python 3.13: %+v", g)
	}
	if g := f["jdk-17"]; g.Status != OK || g.Cycle != "17" || !g.LTS {
		t.Errorf("temurin 17: %+v", g)
	}
	if g := f["jdk-8"]; g.Status != OK || g.Cycle != "8" {
		t.Errorf("legacy java 8 spelling: %+v", g)
	}
	if g := f["jdk-11"]; g.Status != Soon || *g.Days != 174 {
		t.Errorf("temurin 11: %+v", g)
	}
	if g := f["go"]; g.Status != Ended || g.Cycle != "1.25" {
		t.Errorf("go 1.25: %+v", g)
	}
	if g := f["py-other"]; g.Status != Unknown || g.Cycle != "" || g.End != "" || g.Days != nil || g.Source == "" {
		t.Errorf("no version: %+v", g)
	}
	if g := f["py-311"]; g.Status != OK {
		t.Errorf("python 3.11: %+v", g)
	}
}

func TestUnknownWhenTheVersionMatchesNoCycle(t *testing.T) {
	r := data().Build([]Installed{{AppID: "py-310", Version: "3.12.1"}, {AppID: "py-311", Version: "banana"}}, at(2026, 11, 17))
	for _, f := range r.Findings {
		if f.Status != Unknown || f.Cycle != "" || len(f.Replace) != 0 || len(f.Apps) != 0 {
			t.Errorf("%+v", f)
		}
	}
}

func TestAProductPresentTwiceGivesOneFindingPerInstance(t *testing.T) {
	r := data().Build([]Installed{
		{AppID: "jdk-17", Version: "17.0.9"}, {AppID: "jre-17", Version: "17.0.1"}, {AppID: "jdk-11", Version: "11.0.2"}, {AppID: "jdk-8", Version: "8.0.1"},
	}, at(2026, 11, 17))
	if len(r.Findings) != 4 {
		t.Fatalf("%d findings, want 4", len(r.Findings))
	}
	n := 0
	for _, f := range r.Findings {
		if f.Product == "temurin" {
			n++
		}
	}
	if n != 4 {
		t.Errorf("%d temurin findings", n)
	}
}

func TestReplacementsAndAppsOffered(t *testing.T) {
	now := at(2026, 11, 17)
	r := data().Build([]Installed{
		{AppID: "py-310", Version: "3.10.1"},
		{AppID: "jdk-11", Version: "11.0.2"},
		{AppID: "node-20", Version: "20.1.0"},
		{AppID: "node-cur", Version: "25.2.0"},
		{AppID: "go", Version: "1.25.0"},
	}, now)
	f := byApp(r)

	// Python has no long-term lines: the newest supported ones, at most three.
	g := f["py-310"]
	cycles := []string{}
	for _, x := range g.Replace {
		cycles = append(cycles, x.Cycle)
	}
	if !reflect.DeepEqual(cycles, []string{"3.15", "3.14", "3.13"}) || !reflect.DeepEqual(g.Apps, []string{"py-315", "py-314", "py-313"}) {
		t.Errorf("python: %v %v", cycles, g.Apps)
	}
	// A soon-ending Java moves to the long-term lines that will last (17, 21, 25), not to itself.
	g = f["jdk-11"]
	if len(g.Replace) != 3 || g.Replace[0].Cycle != "25" || !g.Replace[0].LTS || g.Replace[0].End != "2031-09-30" {
		t.Errorf("java 11: %+v", g.Replace)
	}
	if !reflect.DeepEqual(g.Apps, []string{"jdk-25", "jdk-21", "jdk-17"}) {
		t.Errorf("java 11 apps: %v", g.Apps)
	}
	// Node 20 (ended): long-term lines that last are 26 and 24 (22 ends in 164 days, so it is not
	// recommended while a longer one exists); the app that tracks LTS is offered too.
	g = f["node-20"]
	if len(g.Replace) != 2 || g.Replace[0].Cycle != "26" || g.Replace[1].Cycle != "24" {
		t.Errorf("node 20: %+v", g.Replace)
	}
	if !reflect.DeepEqual(g.Apps, []string{"node-lts", "node-cur"}) {
		t.Errorf("node 20 apps: %v", g.Apps)
	}
	// An always-newest app is offered for a "latest" follower but is never offered to itself.
	g = f["node-cur"]
	for _, id := range g.Apps {
		if id == "node-cur" {
			t.Errorf("the installed app was offered as its own replacement: %v", g.Apps)
		}
	}
	if g.Status != Ended || g.Cycle != "25" || len(g.Apps) != 1 || g.Apps[0] != "node-lts" {
		t.Errorf("node current: %+v", g)
	}
	// Go: newer supported lines are the two without an end date; the app is already the newest.
	g = f["go"]
	if len(g.Replace) != 2 || g.Replace[0].Cycle != "1.27" || g.Replace[0].End != "" || len(g.Apps) != 0 {
		t.Errorf("go: %+v", g)
	}
}

func TestNoReplacementsForSupportedOrUnknownFindings(t *testing.T) {
	r := data().Build([]Installed{{AppID: "py-313", Version: "3.13.1"}, {AppID: "py-310", Version: "9.9"}}, at(2026, 11, 17))
	for _, f := range r.Findings {
		if len(f.Replace) != 0 || len(f.Apps) != 0 {
			t.Errorf("%+v", f)
		}
	}
	// Replace and Apps are empty lists, never null, for the interface.
	if r.Findings[0].Replace == nil || r.Findings[0].Apps == nil {
		t.Error("lists must not be nil")
	}
}

func TestReportOrdering(t *testing.T) {
	now := at(2026, 11, 17)
	r := data().Build([]Installed{
		{AppID: "py-313", Version: "3.13.1"},  // ok
		{AppID: "py-other", Version: "x"},     // unknown
		{AppID: "jdk-11", Version: "11.0.2"},  // soon, 174 days
		{AppID: "py-310", Version: "3.10.1"},  // ended 47 days ago
		{AppID: "node-20", Version: "20.1.0"}, // ended 201 days ago
		{AppID: "go", Version: "1.27.0"},      // ok, no date
		{AppID: "jdk-17", Version: "17.0.1"},  // ok
		{AppID: "jdk-21", Version: "21.0.1"},  // ok
	}, now)
	var got []string
	for _, f := range r.Findings {
		got = append(got, string(f.Status)+":"+f.AppID)
	}
	// Ok findings go nearest end first; the one with no end date is last.
	want := []string{"ended:node-20", "ended:py-310", "soon:jdk-11", "unknown:py-other", "ok:jdk-17", "ok:py-313", "ok:jdk-21", "ok:go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\n got %v\nwant %v", got, want)
	}
	// The same input in another order gives the same report.
	r2 := data().Build([]Installed{
		{AppID: "jdk-21", Version: "21.0.1"}, {AppID: "jdk-17", Version: "17.0.1"}, {AppID: "go", Version: "1.27.0"}, {AppID: "node-20", Version: "20.1.0"},
		{AppID: "py-310", Version: "3.10.1"}, {AppID: "jdk-11", Version: "11.0.2"}, {AppID: "py-other", Version: "x"}, {AppID: "py-313", Version: "3.13.1"},
	}, now)
	if !reflect.DeepEqual(r.Findings, r2.Findings) {
		t.Error("order depends on the input order")
	}
}

func TestStaleAndAge(t *testing.T) {
	d := &Data{AsOf: "2025-11-17", Products: data().Products}
	cases := []struct {
		now   time.Time
		age   int
		stale bool
	}{
		{at(2025, 11, 17), 0, false},
		{at(2026, 11, 17), 365, false}, // exactly a year: not yet stale
		{at(2026, 11, 18), 366, true},
		{at(2025, 11, 1), 0, false}, // a clock before asOf never gives a negative age
		{at(2029, 1, 1), 1141, true},
	}
	for _, c := range cases {
		r := d.Build(nil, c.now)
		if r.AgeDays != c.age || r.Stale != c.stale || r.AsOf != "2025-11-17" {
			t.Errorf("%s: age %d stale %v, want %d %v", c.now.Format(DayLayout), r.AgeDays, r.Stale, c.age, c.stale)
		}
		if r.Findings == nil {
			t.Error("Findings must be an empty list, not null")
		}
	}
}

func TestValidate(t *testing.T) {
	exists := func(id string) bool { return !strings.HasPrefix(id, "missing") }
	good := func() *Data {
		return &Data{AsOf: "2026-11-17", Products: []Product{{
			ID: "py", Name: "Python", Source: "https://endoflife.date/python", Apps: []string{"a"},
			Cycles: []Cycle{{Cycle: "3.10", EOL: "2026-10-01"}, {Cycle: "3.9", EOL: "2025-10-31"}},
		}}}
	}
	if errs := good().Validate(exists); len(errs) != 0 {
		t.Fatalf("a valid file is rejected: %v", errs)
	}
	bad := []struct {
		name string
		mut  func(*Data)
		msg  string
	}{
		{"bad asOf", func(d *Data) { d.AsOf = "17/11/2026" }, "asOf"},
		{"no products", func(d *Data) { d.Products = nil }, "no products"},
		{"bad id", func(d *Data) { d.Products[0].ID = "Py Thon" }, "id must be"},
		{"no name", func(d *Data) { d.Products[0].Name = " " }, "name is required"},
		{"http source", func(d *Data) { d.Products[0].Source = "http://x" }, "https"},
		{"unknown scheme", func(d *Data) { d.Products[0].Scheme = "semver" }, "scheme"},
		{"no apps", func(d *Data) { d.Products[0].Apps = nil }, "apps is empty"},
		{"unknown app", func(d *Data) { d.Products[0].Apps = []string{"missing-app"} }, "not a catalog app"},
		{"unknown cycle app", func(d *Data) { d.Products[0].Cycles[0].App = "missing-x" }, "not a catalog app"},
		{"unknown follows app", func(d *Data) { d.Products[0].Follows = map[string]string{"missing-y": "lts"} }, "follows"},
		{"bad follows track", func(d *Data) { d.Products[0].Follows = map[string]string{"a": "newest"} }, "track"},
		{"duplicate cycle", func(d *Data) {
			d.Products[0].Cycles = append(d.Products[0].Cycles, Cycle{Cycle: "3.9", EOL: "2025-10-31"})
		}, "twice"},
		{"cycle not numeric", func(d *Data) { d.Products[0].Cycles[0].Cycle = "3.x" }, "dotted numbers"},
		{"bad date", func(d *Data) { d.Products[0].Cycles[0].EOL = "2026-02-30" }, "not a YYYY-MM-DD"},
		{"bad date text", func(d *Data) { d.Products[0].Cycles[1].EOL = "soon" }, "not a YYYY-MM-DD"},
		{"order", func(d *Data) {
			d.Products[0].Cycles[0], d.Products[0].Cycles[1] = d.Products[0].Cycles[1], d.Products[0].Cycles[0]
		}, "newest first"},
		{"no cycles", func(d *Data) { d.Products[0].Cycles = nil }, "no cycles"},
		{"duplicate product", func(d *Data) { d.Products = append(d.Products, d.Products[0]) }, "twice"},
		{"app in two products", func(d *Data) {
			p := d.Products[0]
			p.ID = "other"
			d.Products = append(d.Products, p)
		}, "belongs to products"},
	}
	for _, c := range bad {
		d := good()
		c.mut(d)
		errs := d.Validate(exists)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), c.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want an error containing %q, got %v", c.name, c.msg, errs)
		}
	}
}

func TestParseIsStrict(t *testing.T) {
	ok := "asOf: \"2026-11-17\"\nproducts:\n  - id: a\n    name: A\n    source: \"https://x\"\n    apps: [a]\n    cycles:\n      - {cycle: \"1\", eol: \"2030-01-01\", lts: true}\n"
	d, err := Parse([]byte(ok))
	if err != nil || len(d.Products) != 1 || !d.Products[0].Cycles[0].LTS || d.Products[0].Cycles[0].EOL != "2030-01-01" {
		t.Fatalf("%v %+v", err, d)
	}
	if _, err := Parse([]byte(strings.Replace(ok, "eol:", "end:", 1))); err == nil {
		t.Error("a mistyped field must be an error, not a missing date")
	}
	if _, err := Parse([]byte("asOf: [")); err == nil {
		t.Error("broken YAML must be an error")
	}
}
