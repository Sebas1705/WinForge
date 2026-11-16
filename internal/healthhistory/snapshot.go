package healthhistory

import (
	"math"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/health"
)

// DiskStat is what a drive reports about itself beyond the scan: nil fields are not reported.
type DiskStat struct {
	Name         string `json:"name"`
	WearPct      *int   `json:"wear"`
	TempC        *int   `json:"temp"`
	PowerOnHours *int   `json:"hours"`
}

// Extras is the cheap read-only data that is read alongside a scan.
type Extras struct {
	Disks   []DiskStat
	Battery *battery.Report
}

const gib = float64(1 << 30)

// FromScan boils a scan down to a snapshot. findings must come from health.Analyze without the
// Windows Update search, so the score does not jump when that search is run or not.
func FromScan(r *health.Report, findings []health.Finding, x Extras, now time.Time) Snapshot {
	s := Snapshot{At: now, Areas: map[string]int{}, Volumes: []Volume{}, Disks: []Disk{}}
	ok, total := health.Tally(findings)
	s.Score, s.Total = ok, total
	for _, f := range findings {
		g := string(f.Group)
		if _, seen := s.Areas[g]; !seen {
			s.Areas[g] = 0
		}
		if f.Severity == health.Bad || f.Severity == health.Warn {
			s.Areas[g]++
		}
	}
	if r != nil {
		for _, v := range r.Volumes {
			s.Volumes = append(s.Volumes, Volume{Letter: v.Letter, FreeBytes: bytesOf(v.FreeGB), TotalBytes: bytesOf(v.SizeGB)})
		}
		for _, d := range r.Disks {
			disk := Disk{Name: d.Name, Media: d.Media, Health: d.Health}
			for _, e := range x.Disks {
				if e.Name == d.Name {
					disk.TempC, disk.PowerOnHours = e.TempC, e.PowerOnHours
					if d.Media != "HDD" { // a hard disk has no wear figure; 0 there means "not reported"
						disk.WearPct = e.WearPct
					}
				}
			}
			s.Disks = append(s.Disks, disk)
		}
	}
	if b := x.Battery; b != nil && b.Present && b.DesignMWh > 0 && b.FullMWh > 0 {
		s.Battery = &BatteryPoint{DesignMWh: b.DesignMWh, FullMWh: b.FullMWh, Cycles: b.Cycles}
	}
	return s
}

func bytesOf(gb float64) int64 {
	if math.IsNaN(gb) || math.IsInf(gb, 0) || gb < 0 {
		return -1
	}
	return int64(math.Round(gb * gib))
}
