// Package healthhistory keeps a small, capped record of PC health scans and works out plain trends
// from it. A snapshot is a few numbers (score, problems per area, free space, drive health and wear,
// battery capacity): no serial numbers, no names of files or people. Everything here is pure logic
// plus a store with an in-memory fake; nothing reads the machine except extras.go.
package healthhistory

import (
	"sort"
	"time"
)

const (
	// MaxSnapshots is how many snapshots are kept; the oldest go first.
	MaxSnapshots = 120
	// MinGap is the closest two snapshots may be: a rescan within the hour replaces the last one.
	MinGap = time.Hour

	fileVersion = 1
)

// Volume is one fixed volume's space at the time of the scan. Bytes are approximate: the scan
// reports sizes in tenths of a GB.
type Volume struct {
	Letter     string `json:"letter"`
	FreeBytes  int64  `json:"freeBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

// Disk is one physical drive. Health is Windows' own word (Healthy, Warning, Unhealthy). Wear is
// the share of the drive's rated life used, temperature is in Celsius; both are nil unless the
// drive reports them (reading them usually needs administrator rights).
type Disk struct {
	Name         string `json:"name"`
	Media        string `json:"media,omitempty"`
	Health       string `json:"health,omitempty"`
	WearPct      *int   `json:"wearPct"`
	TempC        *int   `json:"tempC"`
	PowerOnHours *int   `json:"powerOnHours"`
}

// BatteryPoint is one battery reading: capacities in mWh, as the battery reports them.
type BatteryPoint struct {
	DesignMWh int64 `json:"designMWh"`
	FullMWh   int64 `json:"fullMWh"`
	Cycles    *int  `json:"cycles"`
}

// Snapshot is one completed scan, boiled down.
type Snapshot struct {
	At    time.Time `json:"at"`
	Score int       `json:"score"` // checks that passed
	Total int       `json:"total"` // checks that ran
	// Areas maps a health area (firmware, drivers, storage, security, windows) to how many of its
	// checks found a problem (warning or worse).
	Areas   map[string]int `json:"areas"`
	Volumes []Volume       `json:"volumes"`
	Disks   []Disk         `json:"disks"`
	Battery *BatteryPoint  `json:"battery"`
}

// File is what is stored.
type File struct {
	Version   int        `json:"version"`
	Snapshots []Snapshot `json:"snapshots"`
}

func intp(v int) *int { return &v }

// Clean returns the snapshot with every value that cannot be right removed, or ok=false when the
// snapshot itself is unusable. A damaged file can hold anything, so this runs on everything loaded.
func Clean(s Snapshot) (Snapshot, bool) {
	if s.At.IsZero() || s.At.Year() < 2000 || s.Total <= 0 || s.Score < 0 || s.Score > s.Total {
		return Snapshot{}, false
	}
	s.At = s.At.UTC().Truncate(time.Second)
	areas := map[string]int{}
	for k, v := range s.Areas {
		if v >= 0 && v <= 1000 && k != "" {
			areas[k] = v
		}
	}
	s.Areas = areas
	vols := []Volume{}
	for _, v := range s.Volumes {
		if v.Letter != "" && v.TotalBytes > 0 && v.FreeBytes >= 0 && v.FreeBytes <= v.TotalBytes {
			vols = append(vols, v)
		}
	}
	s.Volumes = vols
	disks := []Disk{}
	for _, d := range s.Disks {
		if d.Name == "" {
			continue
		}
		d.WearPct = within(d.WearPct, 0, 100)
		d.TempC = within(d.TempC, 1, 150)
		d.PowerOnHours = within(d.PowerOnHours, 0, 1_000_000)
		disks = append(disks, d)
	}
	s.Disks = disks
	if b := s.Battery; b != nil {
		if b.DesignMWh <= 0 || b.FullMWh <= 0 || b.FullMWh > b.DesignMWh*11/10 {
			s.Battery = nil
		} else {
			c := *b
			c.Cycles = within(c.Cycles, 1, 100000)
			s.Battery = &c
		}
	}
	return s, true
}

func within(p *int, lo, hi int) *int {
	if p == nil || *p < lo || *p > hi {
		return nil
	}
	return intp(*p)
}

// Normalize cleans, sorts and caps a list loaded from disk.
func Normalize(f File) File {
	out := File{Version: fileVersion, Snapshots: []Snapshot{}}
	for _, s := range f.Snapshots {
		if c, ok := Clean(s); ok {
			out.Snapshots = append(out.Snapshots, c)
		}
	}
	sort.SliceStable(out.Snapshots, func(i, j int) bool { return out.Snapshots[i].At.Before(out.Snapshots[j].At) })
	if n := len(out.Snapshots); n > MaxSnapshots {
		out.Snapshots = out.Snapshots[n-MaxSnapshots:]
	}
	return out
}

// Add puts a snapshot in the file and returns the result. A snapshot closer than MinGap to an
// earlier one replaces it (the newest scan wins), keeping what the old one knew and the new one
// does not (a scan without administrator rights cannot read drive wear). The oldest snapshots are
// dropped beyond MaxSnapshots. An unusable snapshot changes nothing.
func Add(f File, s Snapshot) File {
	s, ok := Clean(s)
	if !ok {
		return Normalize(f)
	}
	f = Normalize(f)
	kept := make([]Snapshot, 0, len(f.Snapshots)+1)
	for _, old := range f.Snapshots {
		d := s.At.Sub(old.At)
		if d < 0 {
			d = -d
		}
		if d < MinGap {
			s = fill(s, old)
			continue
		}
		kept = append(kept, old)
	}
	f.Snapshots = append(kept, s)
	return Normalize(f)
}

// fill copies into s the readings that only old has.
func fill(s, old Snapshot) Snapshot {
	if s.Battery == nil {
		s.Battery = old.Battery
	}
	for i := range s.Disks {
		for _, o := range old.Disks {
			if o.Name != s.Disks[i].Name {
				continue
			}
			d := &s.Disks[i]
			if d.WearPct == nil {
				d.WearPct = o.WearPct
			}
			if d.TempC == nil {
				d.TempC = o.TempC
			}
			if d.PowerOnHours == nil {
				d.PowerOnHours = o.PowerOnHours
			}
		}
	}
	return s
}
