package healthhistory

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
)

// reliabilityScript reads each drive's own counters (wear, temperature, power-on hours). It only
// reads, it is quick, and without administrator rights Windows refuses it: then nothing is kept.
const reliabilityScript = `$ErrorActionPreference = 'SilentlyContinue'
$out = @()
foreach ($d in (Get-PhysicalDisk)) {
  $c = $d | Get-StorageReliabilityCounter
  if ($c) { $out += [ordered]@{ name = [string]$d.FriendlyName; wear = $c.Wear; temp = $c.Temperature; hours = $c.PowerOnHours } }
}
ConvertTo-Json -InputObject @($out) -Compress`

// ParseReliability decodes the script's JSON. Values that cannot be a reading are dropped: a
// temperature of 0 means "not reported", and so does a missing field.
func ParseReliability(out []byte) []DiskStat {
	s := strings.TrimSpace(string(out))
	if i := strings.IndexAny(s, "[{"); i > 0 {
		s = s[i:]
	}
	var rows []struct {
		Name  string   `json:"name"`
		Wear  *float64 `json:"wear"`
		Temp  *float64 `json:"temp"`
		Hours *float64 `json:"hours"`
	}
	if err := json.Unmarshal([]byte(s), &rows); err != nil {
		var one struct {
			Name  string   `json:"name"`
			Wear  *float64 `json:"wear"`
			Temp  *float64 `json:"temp"`
			Hours *float64 `json:"hours"`
		}
		if json.Unmarshal([]byte(s), &one) != nil {
			return nil
		}
		rows = append(rows, one)
	}
	conv := func(p *float64, lo, hi int) *int {
		if p == nil {
			return nil
		}
		v := int(*p)
		if *p != *p || v < lo || v > hi { // NaN fails the first test
			return nil
		}
		return &v
	}
	var out2 []DiskStat
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		out2 = append(out2, DiskStat{Name: r.Name, WearPct: conv(r.Wear, 0, 100), TempC: conv(r.Temp, 1, 150), PowerOnHours: conv(r.Hours, 0, 1_000_000)})
	}
	return out2
}

// Collect reads the extras while a scan is running and returns a function that waits for them.
// It never fails: whatever could not be read is left out. The read has its own deadline, so waiting
// for it cannot hold a scan up for long.
func Collect(ctx context.Context, readDisks func(context.Context) ([]DiskStat, error), readBattery func(context.Context) (battery.Report, error)) func() Extras {
	var (
		x  Extras
		wg sync.WaitGroup
	)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if readDisks != nil {
			if d, err := readDisks(ctx); err == nil {
				x.Disks = d
			}
		}
	}()
	go func() {
		defer wg.Done()
		if readBattery != nil {
			if b, err := readBattery(ctx); err == nil && b.Present {
				x.Battery = &b
			}
		}
	}()
	return func() Extras {
		wg.Wait()
		cancel()
		return x
	}
}

// ReadDisks is the real drive-counter read (Windows only; elsewhere it finds nothing).
func ReadDisks(ctx context.Context) ([]DiskStat, error) { return readReliability(ctx) }
