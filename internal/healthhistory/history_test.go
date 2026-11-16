package healthhistory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/health"
)

var t0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func snap(at time.Time, score int) Snapshot {
	return Snapshot{At: at, Score: score, Total: 20, Areas: map[string]int{"storage": 1}}
}

func at(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }

func TestAddKeepsAtMostOnePerHour(t *testing.T) {
	var f File
	f = Add(f, snap(at(0), 15))
	f = Add(f, snap(at(0).Add(30*time.Minute), 16)) // rescan: replaces
	f = Add(f, snap(at(0).Add(59*time.Minute), 17)) // still inside the hour of the kept one? 29 min after: replaces
	if len(f.Snapshots) != 1 || f.Snapshots[0].Score != 17 {
		t.Fatalf("rescans in the same hour must leave one snapshot, the newest: %+v", f.Snapshots)
	}
	f = Add(f, snap(at(0).Add(59*time.Minute+time.Hour), 18)) // exactly an hour after the kept one
	if len(f.Snapshots) != 2 {
		t.Fatalf("an hour later is a new snapshot: %d", len(f.Snapshots))
	}
}

func TestAddCapsAndSorts(t *testing.T) {
	var f File
	for i := 0; i < MaxSnapshots+30; i++ {
		f = Add(f, snap(at(i*2), 10+i%5))
	}
	if len(f.Snapshots) != MaxSnapshots {
		t.Fatalf("cap: %d", len(f.Snapshots))
	}
	if !f.Snapshots[0].At.Equal(at(60)) { // the 30 oldest are gone
		t.Fatalf("oldest kept: %v", f.Snapshots[0].At)
	}
	// A snapshot with an earlier clock lands in order, not at the end.
	f = Add(f, snap(at(61), 3))
	for i := 1; i < len(f.Snapshots); i++ {
		if f.Snapshots[i].At.Before(f.Snapshots[i-1].At) {
			t.Fatal("not sorted")
		}
	}
}

func TestAddIgnoresUnusableSnapshots(t *testing.T) {
	f := Add(File{}, snap(at(0), 15))
	for _, bad := range []Snapshot{
		{}, {At: at(5), Total: 0}, {At: at(5), Total: 20, Score: 21}, {At: at(5), Total: 20, Score: -1},
		{At: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), Total: 20, Score: 5},
	} {
		if g := Add(f, bad); len(g.Snapshots) != 1 {
			t.Fatalf("unusable snapshot %+v was stored", bad)
		}
	}
}

func TestCleanDropsImpossibleValues(t *testing.T) {
	neg, big, ok := -3, 400, 7
	s := snap(at(0), 15)
	s.Volumes = []Volume{
		{Letter: "C", FreeBytes: 10, TotalBytes: 100},
		{Letter: "D", FreeBytes: -1, TotalBytes: 100},
		{Letter: "E", FreeBytes: 101, TotalBytes: 100},
		{Letter: "F", FreeBytes: 1, TotalBytes: 0},
		{Letter: "", FreeBytes: 1, TotalBytes: 2},
	}
	s.Disks = []Disk{{Name: "A", WearPct: &neg, TempC: &big, PowerOnHours: &ok}, {Name: ""}}
	s.Areas = map[string]int{"storage": -2, "drivers": 3}
	s.Battery = &BatteryPoint{DesignMWh: 100, FullMWh: 200}
	c, valid := Clean(s)
	if !valid || len(c.Volumes) != 1 || c.Volumes[0].Letter != "C" {
		t.Fatalf("volumes: %+v", c.Volumes)
	}
	if len(c.Disks) != 1 || c.Disks[0].WearPct != nil || c.Disks[0].TempC != nil || c.Disks[0].PowerOnHours == nil {
		t.Fatalf("disks: %+v", c.Disks)
	}
	if c.Battery != nil || len(c.Areas) != 1 {
		t.Fatalf("battery/areas: %+v %+v", c.Battery, c.Areas)
	}
}

func TestAddKeepsReadingsTheRescanLacks(t *testing.T) {
	wear := 4
	a := snap(at(0), 15)
	a.Disks = []Disk{{Name: "NVMe", WearPct: &wear}}
	a.Battery = &BatteryPoint{DesignMWh: 50000, FullMWh: 45000}
	b := snap(at(0).Add(10*time.Minute), 16) // not elevated: no wear, no battery read
	b.Disks = []Disk{{Name: "NVMe"}}
	f := Add(Add(File{}, a), b)
	if len(f.Snapshots) != 1 || f.Snapshots[0].Score != 16 || f.Snapshots[0].Disks[0].WearPct == nil || f.Snapshots[0].Battery == nil {
		t.Fatalf("%+v", f.Snapshots)
	}
}

func TestFileStoreRoundTripIsAtomic(t *testing.T) {
	dir := t.TempDir()
	s := &FileStore{Path: filepath.Join(dir, "sub", "h.json")}
	f, err := s.Load()
	if err != nil || len(f.Snapshots) != 0 || f.Snapshots == nil {
		t.Fatalf("missing file is an empty history: %v %+v", err, f)
	}
	if err := s.Save(Add(File{}, snap(at(0), 15))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
	g, err := s.Load()
	if err != nil || len(g.Snapshots) != 1 || g.Snapshots[0].Score != 15 {
		t.Fatalf("%v %+v", err, g)
	}
	// A stale temporary file from a crash does not matter: the real file is what is read, and the
	// next save replaces the temporary one.
	if err := os.WriteFile(s.Path+".tmp", []byte("{half"), 0o644); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Load(); len(g.Snapshots) != 1 {
		t.Fatal("a half-written temporary file must be ignored")
	}
	if err := s.Save(Add(g, snap(at(5), 16))); err != nil {
		t.Fatal(err)
	}
	if g, _ := s.Load(); len(g.Snapshots) != 2 {
		t.Fatalf("%+v", g)
	}
}

func TestFileStoreQuarantinesDamagedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"garbage":   "this is not json",
		"truncated": `{"version":1,"snapshots":[{"at":"2026-10-01T09:0`,
		"future":    `{"version":99,"snapshots":[]}`,
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			s := &FileStore{Path: filepath.Join(t.TempDir(), "h.json")}
			if err := os.WriteFile(s.Path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			f, err := s.Load()
			if err != nil || len(f.Snapshots) != 0 {
				t.Fatalf("a damaged file must not fail: %v %+v", err, f)
			}
			if b, err := os.ReadFile(s.Path + ".damaged"); err != nil || string(b) != content {
				t.Fatalf("the damaged file must be kept aside: %v", err)
			}
			if _, err := os.Stat(s.Path); !os.IsNotExist(err) {
				t.Fatal("the damaged file must not stay in place")
			}
			// Scanning goes on: the next record works.
			svc := &Service{Store: s}
			if err := svc.Record(snap(at(0), 15)); err != nil {
				t.Fatal(err)
			}
			if v, _ := svc.View(); len(v.Snapshots) != 1 {
				t.Fatal("history did not restart")
			}
		})
	}
}

func TestFileStoreCleansWhatItLoads(t *testing.T) {
	s := &FileStore{Path: filepath.Join(t.TempDir(), "h.json")}
	good, _ := json.Marshal(snap(at(0), 15))
	bad := `{"at":"2026-10-02T09:00:00Z","score":99,"total":20}`
	raw := `{"version":1,"snapshots":[` + bad + `,` + string(good) + `]}`
	if err := os.WriteFile(s.Path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := s.Load()
	if err != nil || len(f.Snapshots) != 1 {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestFileStoreClear(t *testing.T) {
	s := &FileStore{Path: filepath.Join(t.TempDir(), "h.json")}
	if err := s.Clear(); err != nil {
		t.Fatalf("clearing nothing is fine: %v", err)
	}
	_ = s.Save(Add(File{}, snap(at(0), 15)))
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if f, _ := s.Load(); len(f.Snapshots) != 0 {
		t.Fatal("not cleared")
	}
}

func TestServiceWithMemStore(t *testing.T) {
	m := &MemStore{}
	svc := &Service{Store: m}
	if err := svc.Record(snap(at(0), 15)); err != nil {
		t.Fatal(err)
	}
	m.SaveErr = errors.New("disk full")
	if err := svc.Record(snap(at(5), 15)); err == nil {
		t.Fatal("a failed save must be reported to the caller (which only logs it)")
	}
	m.SaveErr, m.LoadErr = nil, errors.New("locked")
	if err := svc.Record(snap(at(5), 15)); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("%v", err)
	}
	if _, err := svc.View(); err == nil {
		t.Fatal("view must report a failed read")
	}
	m.LoadErr = nil
	if err := svc.Clear(); err != nil {
		t.Fatal(err)
	}
	if v, _ := svc.View(); len(v.Snapshots) != 0 || v.Enough || v.Statements == nil {
		t.Fatalf("%+v", v)
	}
}

// ---- trends

const GiB = int64(1) << 30

func withC(s Snapshot, free, total int64) Snapshot {
	s.Volumes = []Volume{{Letter: "C", FreeBytes: free, TotalBytes: total}}
	return s
}

func codes(st []Statement) []string {
	var c []string
	for _, s := range st {
		c = append(c, s.Code)
	}
	return c
}

func day(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }

func TestTrendsNeedTwoSnapshotsADayApart(t *testing.T) {
	a := withC(snap(day(0), 15), 100*GiB, 500*GiB)
	st, enough := Trends(nil)
	if enough || len(st) != 0 {
		t.Fatal("nothing")
	}
	if st, enough := Trends([]Snapshot{a}); enough || len(st) != 0 {
		t.Fatal("one snapshot says nothing")
	}
	same := withC(snap(day(0).Add(23*time.Hour), 15), 50*GiB, 500*GiB) // a big drop, but under a day
	if st, enough := Trends([]Snapshot{a, same}); enough || len(st) != 0 {
		t.Fatalf("less than a day apart must say nothing: %+v", st)
	}
	next := withC(snap(day(0).Add(24*time.Hour), 15), 50*GiB, 500*GiB)
	if st, enough := Trends([]Snapshot{a, next}); !enough || len(st) != 1 || st[0].Code != StmtFreeFell {
		t.Fatalf("exactly a day apart is enough: %+v", st)
	}
}

func TestTrendFreeSpaceFell(t *testing.T) {
	a := withC(snap(day(0), 15), 200*GiB, 500*GiB)
	b := withC(snap(day(30), 15), 188*GiB, 500*GiB)
	st, enough := Trends([]Snapshot{a, b})
	if !enough || len(st) != 1 {
		t.Fatalf("%+v", st)
	}
	p := st[0].Params
	if st[0].Code != StmtFreeFell || p["drive"] != "C" || p["bytes"] != "12884901888" || p["days"] != "30" {
		t.Fatalf("%+v", st[0])
	}
	rose, _ := Trends([]Snapshot{b, withC(snap(day(31), 15), 300*GiB, 500*GiB)})
	if len(rose) != 1 || rose[0].Code != StmtFreeRose {
		t.Fatalf("%+v", rose)
	}
}

func TestTrendIgnoresNoise(t *testing.T) {
	// Under 1 GiB, and under 1 % of a 4 TB drive (40 GiB): both are noise.
	for _, c := range []struct{ free, total int64 }{{200*GiB - GiB/2, 500 * GiB}, {200*GiB - 10*GiB, 4000 * GiB}} {
		b := withC(snap(day(5), 15), c.free, c.total)
		aa := withC(snap(day(0), 15), 200*GiB, c.total)
		if st, _ := Trends([]Snapshot{aa, b}); len(st) != 1 || st[0].Code != StmtSteady {
			t.Fatalf("%+v should be steady: %+v", c, st)
		}
	}
}

func TestTrendResizedOrMissingVolumeIsSkipped(t *testing.T) {
	a := withC(snap(day(0), 15), 400*GiB, 500*GiB)
	b := withC(snap(day(5), 15), 100*GiB, 250*GiB) // shrunk: free space is not comparable
	if st, _ := Trends([]Snapshot{a, b}); len(st) != 1 || st[0].Code != StmtSteady {
		t.Fatalf("%+v", st)
	}
	c := snap(day(5), 15)
	c.Volumes = []Volume{{Letter: "D", FreeBytes: 1 * GiB, TotalBytes: 500 * GiB}} // D is new
	if st, _ := Trends([]Snapshot{a, c}); len(st) != 1 || st[0].Code != StmtSteady {
		t.Fatalf("%+v", st)
	}
}

func TestTrendUsesOldestInThirtyDaysAndOnlyThat(t *testing.T) {
	old := withC(snap(day(0), 15), 400*GiB, 500*GiB)
	mid := withC(snap(day(40), 15), 300*GiB, 500*GiB)
	last := withC(snap(day(50), 15), 290*GiB, 500*GiB)
	st, _ := Trends([]Snapshot{old, mid, last})
	if len(st) != 1 || st[0].Code != StmtFreeFell || st[0].Params["bytes"] != "10737418240" || st[0].Params["days"] != "10" {
		t.Fatalf("the 50-day-old reading is outside the window: %+v", st)
	}
}

func TestTrendMissingFields(t *testing.T) {
	// Snapshots from an older app version have no volumes, disks or battery at all.
	a, b := snap(day(0), 15), snap(day(10), 15)
	st, enough := Trends([]Snapshot{a, b})
	if !enough || len(st) != 1 || st[0].Code != StmtSteady || st[0].Params["days"] != "10" {
		t.Fatalf("%+v", st)
	}
}

func TestTrendWear(t *testing.T) {
	w := func(n int) *int { return &n }
	mk := func(d int, wear *int) Snapshot {
		s := snap(day(d), 15)
		s.Disks = []Disk{{Name: "NVMe", Media: "SSD", Health: "Healthy", WearPct: wear}}
		return s
	}
	st, _ := Trends([]Snapshot{mk(0, w(4)), mk(20, w(6))})
	if len(st) != 1 || st[0].Code != StmtWearRose || st[0].Params["from"] != "4" || st[0].Params["to"] != "6" || st[0].Params["disk"] != "NVMe" {
		t.Fatalf("%+v", st)
	}
	if st, _ := Trends([]Snapshot{mk(0, w(4)), mk(20, w(4))}); st[0].Code != StmtSteady {
		t.Fatalf("same wear: %+v", st)
	}
	if st, _ := Trends([]Snapshot{mk(0, w(6)), mk(20, w(5))}); st[0].Code != StmtSteady {
		t.Fatalf("a lower reading is not worth a statement: %+v", st)
	}
	// Wear only known in the later scan (the first was not elevated): no comparison.
	if st, _ := Trends([]Snapshot{mk(0, nil), mk(20, w(6))}); st[0].Code != StmtSteady {
		t.Fatalf("%+v", st)
	}
	// The oldest snapshot without a reading is skipped in favour of the oldest with one.
	if st, _ := Trends([]Snapshot{mk(0, nil), mk(5, w(3)), mk(20, w(6))}); st[0].Code != StmtWearRose || st[0].Params["from"] != "3" || st[0].Params["days"] != "15" {
		t.Fatalf("%+v", st)
	}
}

func TestTrendDiskHealth(t *testing.T) {
	mk := func(d int, h string) Snapshot {
		s := snap(day(d), 15)
		s.Disks = []Disk{{Name: "Disk0", Health: h}}
		return s
	}
	st, _ := Trends([]Snapshot{mk(0, "Healthy"), mk(3, "Warning")})
	if len(st) != 1 || st[0].Code != StmtDiskHealth || st[0].Params["to"] != "Warning" {
		t.Fatalf("%+v", st)
	}
	if st, _ := Trends([]Snapshot{mk(0, "Warning"), mk(3, "Healthy")}); st[0].Code != StmtSteady {
		t.Fatalf("recovering says nothing alarming: %+v", st)
	}
	if st, _ := Trends([]Snapshot{mk(0, ""), mk(3, "Warning")}); st[0].Code != StmtSteady {
		t.Fatalf("no earlier reading, no claim: %+v", st)
	}
}

func TestTrendBattery(t *testing.T) {
	mk := func(d int, full int64) Snapshot {
		s := snap(day(d), 15)
		s.Battery = &BatteryPoint{DesignMWh: 50000, FullMWh: full}
		return s
	}
	st, _ := Trends([]Snapshot{mk(0, 45000), mk(30, 42500)}) // 90 % -> 85 %
	if len(st) != 1 || st[0].Code != StmtBatteryFell || st[0].Params["from"] != "90" || st[0].Params["to"] != "85" {
		t.Fatalf("%+v", st)
	}
	if st, _ := Trends([]Snapshot{mk(0, 45000), mk(30, 44500)}); st[0].Code != StmtSteady { // 1 point: noise
		t.Fatalf("%+v", st)
	}
	other := mk(30, 30000)
	other.Battery.DesignMWh = 40000 // a different pack (or a bad read): not comparable
	if st, _ := Trends([]Snapshot{mk(0, 45000), other}); st[0].Code != StmtSteady {
		t.Fatalf("%+v", st)
	}
}

func TestTrendScore(t *testing.T) {
	a, b := snap(day(0), 18), snap(day(7), 15)
	if st, _ := Trends([]Snapshot{a, b}); len(st) != 1 || st[0].Code != StmtScoreFell || st[0].Params["from"] != "18" || st[0].Params["to"] != "15" {
		t.Fatalf("%+v", st)
	}
	if st, _ := Trends([]Snapshot{snap(day(0), 15), snap(day(7), 18)}); st[0].Code != StmtScoreRose {
		t.Fatalf("%+v", st)
	}
	if st, _ := Trends([]Snapshot{snap(day(0), 15), snap(day(7), 16)}); st[0].Code != StmtSteady {
		t.Fatalf("one check is noise: %+v", st)
	}
	c := snap(day(7), 10)
	c.Total = 12 // admin changes how many checks can run: scores are not comparable
	if st, _ := Trends([]Snapshot{snap(day(0), 15), c}); st[0].Code != StmtSteady {
		t.Fatalf("%+v", st)
	}
}

func TestTrendsNeverAlarmOnBadStoredData(t *testing.T) {
	// Whatever a damaged file holds goes through Normalize first (BuildView), so negative or
	// impossible numbers never become statements.
	a := withC(snap(day(0), 15), 100*GiB, 500*GiB)
	b := withC(snap(day(9), 15), -50*GiB, 500*GiB)
	v := BuildView(File{Version: 1, Snapshots: []Snapshot{a, b}})
	if len(v.Snapshots) != 2 || len(v.Snapshots[1].Volumes) != 0 {
		t.Fatalf("%+v", v.Snapshots[1])
	}
	for _, s := range v.Statements {
		if s.Code != StmtSteady {
			t.Fatalf("%+v", s)
		}
	}
}

// ---- snapshot from a scan

func TestFromScan(t *testing.T) {
	r := &health.Report{
		Disks:   []health.Disk{{Name: "Samsung SSD", Media: "SSD", Health: "Healthy"}, {Name: "WD Blue", Media: "HDD", Health: "Healthy"}},
		Volumes: []health.Volume{{Letter: "C", SizeGB: 476.3, FreeGB: 120.5}, {Letter: "D", SizeGB: -1, FreeGB: 1}},
	}
	fs := []health.Finding{
		{Key: "disk-space", Group: health.GroupStorage, Severity: health.Warn},
		{Key: "disk-health", Group: health.GroupStorage, Severity: health.OK},
		{Key: "bios-age", Group: health.GroupFirmware, Severity: health.Bad},
		{Key: "tpm", Group: health.GroupFirmware, Severity: health.Unknown},
		{Key: "uefi-mode", Group: health.GroupFirmware, Severity: health.Info},
	}
	wear, hdd := 6, 3
	temp := 41
	x := Extras{Disks: []DiskStat{{Name: "Samsung SSD", WearPct: &wear, TempC: &temp}, {Name: "WD Blue", WearPct: &hdd}},
		Battery: &battery.Report{Present: true, DesignMWh: 50000, FullMWh: 40000}}
	s := FromScan(r, fs, x, at(0))
	if s.Score != 1 || s.Total != 4 || s.Areas["storage"] != 1 || s.Areas["firmware"] != 1 {
		t.Fatalf("%+v", s)
	}
	if s.Volumes[0].FreeBytes != int64(120.5*float64(GiB)) || s.Volumes[0].TotalBytes <= 0 {
		t.Fatalf("%+v", s.Volumes)
	}
	if s.Disks[0].WearPct == nil || *s.Disks[0].WearPct != 6 || *s.Disks[0].TempC != 41 {
		t.Fatalf("%+v", s.Disks[0])
	}
	if s.Disks[1].WearPct != nil {
		t.Fatal("a hard disk has no wear figure")
	}
	if s.Battery == nil || s.Battery.FullMWh != 40000 {
		t.Fatalf("%+v", s.Battery)
	}
	// Whatever FromScan builds must survive Clean, bad volume dropped.
	c, ok := Clean(s)
	if !ok || len(c.Volumes) != 1 {
		t.Fatalf("%+v", c)
	}
	// No report and no extras still gives a usable snapshot of the checks.
	if s := FromScan(nil, fs, Extras{}, at(0)); s.Total != 4 || len(s.Volumes) != 0 {
		t.Fatalf("%+v", s)
	}
	// A desktop (battery report without a battery) records no battery.
	if s := FromScan(r, fs, Extras{Battery: &battery.Report{}}, at(0)); s.Battery != nil {
		t.Fatal("no battery on a desktop")
	}
}

func TestParseReliability(t *testing.T) {
	got := ParseReliability([]byte(`[{"name":"Samsung SSD 980","wear":6,"temp":41,"hours":1234},{"name":"WD Blue","wear":0,"temp":0,"hours":null},{"name":"","wear":1}]`))
	if len(got) != 2 || *got[0].WearPct != 6 || *got[0].TempC != 41 || *got[0].PowerOnHours != 1234 {
		t.Fatalf("%+v", got)
	}
	if got[1].TempC != nil || got[1].PowerOnHours != nil || got[1].WearPct == nil {
		t.Fatalf("0 °C is not a reading; 0 wear is: %+v", got[1])
	}
	one := ParseReliability([]byte("\xef\xbb\xbf" + `{"name":"Only","wear":2,"temp":30,"hours":5}`))
	if len(one) != 1 || *one[0].WearPct != 2 {
		t.Fatalf("a single object (older PowerShell) must work: %+v", one)
	}
	for _, in := range []string{"", "[]", "garbage", `[{"name":"x","wear":-5,"temp":9999,"hours":-1}]`} {
		for _, d := range ParseReliability([]byte(in)) {
			if d.WearPct != nil || d.TempC != nil || d.PowerOnHours != nil {
				t.Fatalf("%q: impossible values must be dropped: %+v", in, d)
			}
		}
	}
}

func TestCollectWaitsAndSurvivesFailures(t *testing.T) {
	wear := 5
	wait := Collect(context.Background(),
		func(context.Context) ([]DiskStat, error) { return []DiskStat{{Name: "A", WearPct: &wear}}, nil },
		func(context.Context) (battery.Report, error) {
			return battery.Report{Present: true, DesignMWh: 1, FullMWh: 1}, nil
		})
	x := wait()
	if len(x.Disks) != 1 || x.Battery == nil {
		t.Fatalf("%+v", x)
	}
	wait = Collect(context.Background(),
		func(context.Context) ([]DiskStat, error) { return nil, errors.New("access denied") },
		func(context.Context) (battery.Report, error) { return battery.Report{}, errors.New("no powercfg") })
	if x := wait(); x.Disks != nil || x.Battery != nil {
		t.Fatalf("failures leave the extras out: %+v", x)
	}
	wait = Collect(context.Background(), nil, func(context.Context) (battery.Report, error) { return battery.Report{}, nil })
	if x := wait(); x.Battery != nil {
		t.Fatal("a desktop has no battery")
	}
}
