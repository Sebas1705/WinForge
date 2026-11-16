package healthhistory

import (
	"fmt"
	"math"
	"time"
)

// Trend statement codes. The interface words them (hh.stmt.<code>) from Params; nothing here is prose.
const (
	StmtFreeFell    = "free-fell"    // drive, bytes, days
	StmtFreeRose    = "free-rose"    // drive, bytes, days
	StmtWearRose    = "wear-rose"    // disk, from, to, days
	StmtBatteryFell = "battery-fell" // from, to (percent of design), days
	StmtScoreFell   = "score-fell"   // from, to, total, days
	StmtScoreRose   = "score-rose"   // from, to, total, days
	StmtDiskHealth  = "disk-health"  // disk, from, to, days (Windows' own words)
	StmtSteady      = "steady"       // days: enough data, nothing worth saying
)

// Rules. A change smaller than these is noise and is not mentioned.
const (
	MinSpan        = 24 * time.Hour      // the two readings compared must be at least this far apart
	Window         = 30 * 24 * time.Hour // compare against the oldest reading in the last 30 days
	MinFreeBytes   = int64(1) << 30      // free space must move by at least 1 GiB ...
	MinFreeShare   = 0.01                // ... and by 1 % of the volume
	MinWearPoints  = 1                   // wear is a whole percent
	MinBatteryDrop = 2.0                 // battery health points
	MinScoreChange = 2                   // checks
)

// Statement is one plain fact about how the PC changed.
type Statement struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params"`
}

func stmt(code string, kv ...string) Statement {
	p := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		p[kv[i]] = kv[i+1]
	}
	return Statement{Code: code, Params: p}
}

func days(a, b time.Time) string {
	d := int(math.Round(b.Sub(a).Hours() / 24))
	if d < 1 {
		d = 1
	}
	return fmt.Sprint(d)
}

// Trends compares the newest snapshot with the oldest one in the last 30 days that has the same
// reading, per reading. snaps must be sorted by time (Normalize does it). enough is false when
// there are not two snapshots a day apart; then there are no statements at all. When there is
// enough data but nothing changed beyond the noise limits, the only statement is "steady".
func Trends(snaps []Snapshot) (out []Statement, enough bool) {
	out = []Statement{}
	if len(snaps) < 2 {
		return out, false
	}
	last := snaps[len(snaps)-1]
	// base picks the oldest snapshot in the window, at least MinSpan before last, that has a
	// reading (has says so); nil when there is none.
	base := func(has func(Snapshot) bool) *Snapshot {
		for i := range snaps[:len(snaps)-1] {
			s := &snaps[i]
			if last.At.Sub(s.At) > Window || last.At.Sub(s.At) < MinSpan || !has(*s) {
				continue
			}
			return s
		}
		return nil
	}
	if base(func(Snapshot) bool { return true }) == nil {
		return out, false
	}
	enough = true

	if b := base(func(s Snapshot) bool { return s.Total == last.Total }); b != nil {
		switch delta := last.Score - b.Score; {
		case delta <= -MinScoreChange:
			out = append(out, stmt(StmtScoreFell, "from", fmt.Sprint(b.Score), "to", fmt.Sprint(last.Score), "total", fmt.Sprint(last.Total), "days", days(b.At, last.At)))
		case delta >= MinScoreChange:
			out = append(out, stmt(StmtScoreRose, "from", fmt.Sprint(b.Score), "to", fmt.Sprint(last.Score), "total", fmt.Sprint(last.Total), "days", days(b.At, last.At)))
		}
	}

	for _, v := range last.Volumes {
		b := base(func(s Snapshot) bool { _, ok := volume(s, v.Letter); return ok })
		if b == nil {
			continue
		}
		bv, _ := volume(*b, v.Letter)
		if bv.TotalBytes != v.TotalBytes && math.Abs(float64(bv.TotalBytes-v.TotalBytes)) > 0.02*float64(v.TotalBytes) {
			continue // the volume was resized: free space is not comparable
		}
		delta := v.FreeBytes - bv.FreeBytes
		limit := int64(math.Max(float64(MinFreeBytes), MinFreeShare*float64(v.TotalBytes)))
		switch {
		case delta <= -limit:
			out = append(out, stmt(StmtFreeFell, "drive", v.Letter, "bytes", fmt.Sprint(-delta), "days", days(b.At, last.At)))
		case delta >= limit:
			out = append(out, stmt(StmtFreeRose, "drive", v.Letter, "bytes", fmt.Sprint(delta), "days", days(b.At, last.At)))
		}
	}

	for _, d := range last.Disks {
		if d.WearPct != nil {
			if b := base(func(s Snapshot) bool { o, ok := disk(s, d.Name); return ok && o.WearPct != nil }); b != nil {
				bd, _ := disk(*b, d.Name)
				if *d.WearPct-*bd.WearPct >= MinWearPoints {
					out = append(out, stmt(StmtWearRose, "disk", d.Name, "from", fmt.Sprint(*bd.WearPct), "to", fmt.Sprint(*d.WearPct), "days", days(b.At, last.At)))
				}
			}
		}
		if d.Health != "" {
			if b := base(func(s Snapshot) bool { o, ok := disk(s, d.Name); return ok && o.Health != "" }); b != nil {
				bd, _ := disk(*b, d.Name)
				if bd.Health != d.Health && d.Health != "Healthy" {
					out = append(out, stmt(StmtDiskHealth, "disk", d.Name, "from", bd.Health, "to", d.Health, "days", days(b.At, last.At)))
				}
			}
		}
	}

	if lb := last.Battery; lb != nil {
		if b := base(func(s Snapshot) bool { return s.Battery != nil && s.Battery.DesignMWh == lb.DesignMWh }); b != nil {
			from := float64(b.Battery.FullMWh) / float64(lb.DesignMWh) * 100
			to := float64(lb.FullMWh) / float64(lb.DesignMWh) * 100
			if from-to >= MinBatteryDrop {
				out = append(out, stmt(StmtBatteryFell, "from", fmt.Sprint(int(math.Round(from))), "to", fmt.Sprint(int(math.Round(to))), "days", days(b.At, last.At)))
			}
		}
	}

	if len(out) == 0 {
		first := base(func(Snapshot) bool { return true })
		out = append(out, stmt(StmtSteady, "days", days(first.At, last.At)))
	}
	return out, enough
}

func volume(s Snapshot, letter string) (Volume, bool) {
	for _, v := range s.Volumes {
		if v.Letter == letter {
			return v, true
		}
	}
	return Volume{}, false
}

func disk(s Snapshot, name string) (Disk, bool) {
	for _, d := range s.Disks {
		if d.Name == name {
			return d, true
		}
	}
	return Disk{}, false
}
