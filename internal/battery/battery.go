// Package battery reads the laptop battery's condition the way Windows reports it: design capacity
// against what a full charge holds now. It only reads (powercfg writes its report to a temporary
// file that is deleted straight away) and it never claims more than the report says: no
// serial number or battery id is kept, and a reading that does not add up is "unknown".
package battery

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Verdicts. The interface words them in the person's language.
const (
	VerdictGood    = "good"    // holds most of its original charge
	VerdictWorn    = "worn"    // noticeably worn, still usable
	VerdictReplace = "replace" // a full charge lasts a lot less than when new
	VerdictUnknown = "unknown" // the report does not give a usable reading
)

// Thresholds, in percent of the design capacity that a full charge still holds. They live here and
// nowhere else; Verdict is the only function that applies them.
const (
	GoodMinPct   = 80.0 // at or above: good
	ReplaceBelow = 60.0 // below: replace soon; in between: worn

	maxPlausible  = 110.0 // a full charge this far above the design is a bad reading
	historyPoints = 12
)

// Battery is one battery in the report. Serial numbers and ids are deliberately not read.
type Battery struct {
	Chemistry string  `json:"chemistry,omitempty"`
	DesignMWh int64   `json:"designMWh"`
	FullMWh   int64   `json:"fullMWh"`
	HealthPct float64 `json:"healthPct"` // 0 when the verdict is unknown
	Cycles    *int    `json:"cycles"`    // nil when the battery does not report it
	Verdict   string  `json:"verdict"`
}

// Point is one weekly reading from the report's history.
type Point struct {
	Date      string  `json:"date"` // yyyy-mm-dd
	DesignMWh int64   `json:"designMWh"`
	FullMWh   int64   `json:"fullMWh"`
	HealthPct float64 `json:"healthPct"`
	Cycles    *int    `json:"cycles"`
}

// Report is what one battery read found. Present is false on a PC without a battery.
type Report struct {
	Present   bool      `json:"present"`
	Batteries []Battery `json:"batteries"`
	// DesignMWh, FullMWh, HealthPct, Cycles and Verdict describe all batteries together.
	DesignMWh int64   `json:"designMWh"`
	FullMWh   int64   `json:"fullMWh"`
	HealthPct float64 `json:"healthPct"`
	Cycles    *int    `json:"cycles"`
	Verdict   string  `json:"verdict"`
	// History is the recent weekly capacity readings, oldest first; empty when there are none.
	History []Point `json:"history"`
	// GoodMinPct and ReplaceBelowPct are the verdict thresholds, so the interface can state them
	// without keeping its own copy.
	GoodMinPct      float64 `json:"goodMinPct"`
	ReplaceBelowPct float64 `json:"replaceBelowPct"`
}

// Health returns the share of the design capacity a full charge holds, rounded to one decimal.
// ok is false when the numbers cannot be trusted: no design capacity, no full-charge reading, or a
// full charge far above the design (some firmware reports garbage).
func Health(design, full int64) (pct float64, ok bool) {
	if design <= 0 || full <= 0 {
		return 0, false
	}
	p := float64(full) / float64(design) * 100
	if math.IsNaN(p) || math.IsInf(p, 0) || p > maxPlausible {
		return 0, false
	}
	if p > 100 {
		p = 100 // a few packs hold slightly more than the label when new
	}
	return math.Round(p*10) / 10, true
}

// Verdict applies the thresholds.
func Verdict(design, full int64) string {
	if _, ok := Health(design, full); !ok {
		return VerdictUnknown
	}
	p := float64(full) / float64(design) * 100 // unrounded: 79.96 % is not "80"
	switch {
	case p >= GoodMinPct:
		return VerdictGood
	case p >= ReplaceBelow:
		return VerdictWorn
	}
	return VerdictReplace
}

type xmlReport struct {
	Batteries struct {
		Battery []struct {
			Chemistry          string `xml:"Chemistry"`
			DesignCapacity     string `xml:"DesignCapacity"`
			FullChargeCapacity string `xml:"FullChargeCapacity"`
			CycleCount         string `xml:"CycleCount"`
		} `xml:"Battery"`
	} `xml:"Batteries"`
	History struct {
		Entry []struct {
			StartDate          string `xml:"StartDate,attr"`
			DesignCapacity     string `xml:"DesignCapacity,attr"`
			FullChargeCapacity string `xml:"FullChargeCapacity,attr"`
			CycleCount         string `xml:"CycleCount,attr"`
		} `xml:"HistoryEntry"`
	} `xml:"History"`
}

// Parse reads the XML report that `powercfg /batteryreport /xml` writes. A report without
// batteries (a desktop) is a valid report with Present false.
func Parse(data []byte) (Report, error) {
	data, err := toUTF8(data)
	if err != nil {
		return Report{}, err
	}
	var x xmlReport
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	if err := dec.Decode(&x); err != nil {
		return Report{}, fmt.Errorf("could not read the battery report: %w", err)
	}
	r := Report{Batteries: []Battery{}, History: []Point{}, Verdict: VerdictUnknown, GoodMinPct: GoodMinPct, ReplaceBelowPct: ReplaceBelow}
	var design, full int64
	var cycles *int
	allKnown := true
	for _, b := range x.Batteries.Battery {
		d, f := atoi(b.DesignCapacity), atoi(b.FullChargeCapacity)
		bat := Battery{Chemistry: strings.TrimSpace(b.Chemistry), DesignMWh: d, FullMWh: f, Cycles: cyclesOf(b.CycleCount), Verdict: Verdict(d, f)}
		if p, ok := Health(d, f); ok {
			bat.HealthPct = p
		} else {
			allKnown = false
		}
		r.Batteries = append(r.Batteries, bat)
		design += d
		full += f
		if bat.Cycles != nil && (cycles == nil || *bat.Cycles > *cycles) {
			c := *bat.Cycles
			cycles = &c
		}
	}
	if len(r.Batteries) == 0 {
		return r, nil
	}
	r.Present = true
	r.Cycles = cycles
	if allKnown {
		r.DesignMWh, r.FullMWh = design, full
		r.HealthPct, _ = Health(design, full)
		r.Verdict = Verdict(design, full)
	}
	for _, e := range x.History.Entry {
		d, f := atoi(e.DesignCapacity), atoi(e.FullChargeCapacity)
		p, ok := Health(d, f)
		if !ok || len(e.StartDate) < 10 {
			continue
		}
		r.History = append(r.History, Point{Date: e.StartDate[:10], DesignMWh: d, FullMWh: f, HealthPct: p, Cycles: cyclesOf(e.CycleCount)})
	}
	if n := len(r.History); n > historyPoints {
		r.History = r.History[n-historyPoints:]
	}
	return r, nil
}

func atoi(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// cyclesOf treats 0 as "not reported": most batteries that do not count cycles say 0.
func cyclesOf(s string) *int {
	n := atoi(s)
	if n <= 0 || n > 100000 {
		return nil
	}
	c := int(n)
	return &c
}

// toUTF8 drops a byte order mark and converts a UTF-16 report; the XML parser then reads UTF-8.
func toUTF8(b []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(b, []byte("\xef\xbb\xbf")):
		return b[3:], nil
	case bytes.HasPrefix(b, []byte{0xff, 0xfe}), bytes.HasPrefix(b, []byte{0xfe, 0xff}):
		var order binary.ByteOrder = binary.LittleEndian
		if b[0] == 0xfe {
			order = binary.BigEndian
		}
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = order.Uint16(b[2*i:])
		}
		s := string(utf16.Decode(u))
		if i := strings.Index(s, "?>"); strings.HasPrefix(s, "<?xml") && i > 0 {
			s = s[i+2:] // the declaration names the old encoding
		}
		return []byte(s), nil
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("the battery report is empty")
	}
	return b, nil
}
