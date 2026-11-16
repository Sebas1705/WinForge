package battery

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// A trimmed report in the shape powercfg writes for a laptop (ids and serials are made up).
const laptopXML = `<?xml version="1.0" encoding="UTF-8"?>
<BatteryReport xmlns="http://schemas.microsoft.com/battery/2012">
<ReportInformation><ReportVersion>1.1</ReportVersion><ScanTime>2026-11-16T19:02:56Z</ScanTime></ReportInformation>
<SystemInformation><ComputerName>LAPTOP-TEST</ComputerName><PlatformRole>Mobile</PlatformRole></SystemInformation>
<Batteries>
<Battery>
<Id>DELL XYZ1234</Id><Manufacturer>SMP</Manufacturer><SerialNumber>0123</SerialNumber>
<Chemistry>LiP</Chemistry><LongTerm>1</LongTerm><RelativeCapacity>0</RelativeCapacity>
<DesignCapacity>56000</DesignCapacity><FullChargeCapacity>%d</FullChargeCapacity><CycleCount>%d</CycleCount>
</Battery>
</Batteries>
<History>
<HistoryEntry StartDate="2026-09-28T10:00:00Z" EndDate="2026-10-05T10:00:00Z" DesignCapacity="56000" FullChargeCapacity="49000" CycleCount="301" />
<HistoryEntry StartDate="2026-10-05T10:00:00Z" EndDate="2026-10-12T10:00:00Z" DesignCapacity="56000" FullChargeCapacity="0" CycleCount="0" />
<HistoryEntry StartDate="2026-10-12T10:00:00Z" EndDate="2026-10-19T10:00:00Z" DesignCapacity="56000" FullChargeCapacity="48500" CycleCount="305" />
</History>
</BatteryReport>`

func laptop(full, cycles int) []byte { return []byte(fmt.Sprintf(laptopXML, full, cycles)) }

const desktopXML = `<?xml version="1.0" encoding="UTF-8"?>
<BatteryReport xmlns="http://schemas.microsoft.com/battery/2012">
<SystemInformation><PlatformRole>Desktop</PlatformRole></SystemInformation>
<Batteries>
</Batteries>
<RuntimeEstimates><DesignCapacity><Capacity>0</Capacity></DesignCapacity></RuntimeEstimates>
<History><HistoryEntry StartDate="2026-07-20T14:48:07Z" DesignCapacity="0" FullChargeCapacity="0" CycleCount="0" /></History>
</BatteryReport>`

func TestParseLaptop(t *testing.T) {
	r, err := Parse(laptop(48200, 312))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Present || len(r.Batteries) != 1 {
		t.Fatalf("want one battery, got %+v", r)
	}
	if r.DesignMWh != 56000 || r.FullMWh != 48200 || r.HealthPct != 86.1 || r.Verdict != VerdictGood {
		t.Fatalf("totals wrong: %+v", r)
	}
	if r.GoodMinPct != GoodMinPct || r.ReplaceBelowPct != ReplaceBelow {
		t.Fatalf("the report must carry the thresholds the verdict used: %+v", r)
	}
	if r.Cycles == nil || *r.Cycles != 312 || r.Batteries[0].Chemistry != "LiP" {
		t.Fatalf("cycles/chemistry wrong: %+v", r)
	}
	// The zero full-charge entry is skipped; the other two are kept, oldest first.
	if len(r.History) != 2 || r.History[0].Date != "2026-09-28" || r.History[0].HealthPct != 87.5 || r.History[1].HealthPct != 86.6 {
		t.Fatalf("history wrong: %+v", r.History)
	}
}

func TestParseKeepsNoIdentity(t *testing.T) {
	r, _ := Parse(laptop(48200, 312))
	if s := fmt.Sprintf("%+v", r); strings.Contains(s, "DELL") || strings.Contains(s, "0123") || strings.Contains(s, "SMP") {
		t.Fatalf("battery id, maker or serial leaked: %s", s)
	}
}

func TestParseDesktopHasNoBattery(t *testing.T) {
	r, err := Parse([]byte(desktopXML))
	if err != nil {
		t.Fatal(err)
	}
	if r.Present || len(r.Batteries) != 0 || r.Verdict != VerdictUnknown || len(r.History) != 0 {
		t.Fatalf("a desktop has no battery: %+v", r)
	}
	if r.Batteries == nil || r.History == nil {
		t.Fatal("slices must not be nil (JSON null)")
	}
}

func TestVerdictThresholds(t *testing.T) {
	cases := []struct {
		full int64
		want string
	}{
		{56000, VerdictGood}, // 100 %
		{44800, VerdictGood}, // exactly 80 %
		{44799, VerdictWorn}, // just under
		{33600, VerdictWorn}, // exactly 60 %
		{33599, VerdictReplace},
		{10000, VerdictReplace},
		{0, VerdictUnknown},     // no reading
		{61000, VerdictGood},    // 108 %: clamped, plausible
		{70000, VerdictUnknown}, // 125 %: a bad reading, not a good battery
	}
	for _, c := range cases {
		if got := Verdict(56000, c.full); got != c.want {
			t.Errorf("full %d: %s, want %s", c.full, got, c.want)
		}
	}
	if Verdict(0, 1000) != VerdictUnknown || Verdict(-5, -5) != VerdictUnknown {
		t.Error("no design capacity must be unknown")
	}
	if p, _ := Health(56000, 61000); p != 100 {
		t.Errorf("clamp: %v", p)
	}
}

func TestParseTwoBatteriesAndMissingCycles(t *testing.T) {
	x := `<BatteryReport><Batteries>
<Battery><DesignCapacity>30000</DesignCapacity><FullChargeCapacity>27000</FullChargeCapacity><CycleCount>0</CycleCount></Battery>
<Battery><DesignCapacity>20000</DesignCapacity><FullChargeCapacity>10000</FullChargeCapacity><CycleCount>150</CycleCount></Battery>
</Batteries></BatteryReport>`
	r, err := Parse([]byte(x))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Batteries) != 2 || r.Batteries[0].Cycles != nil || r.Batteries[1].Cycles == nil {
		t.Fatalf("0 cycles means not reported: %+v", r.Batteries)
	}
	if r.DesignMWh != 50000 || r.FullMWh != 37000 || r.HealthPct != 74 || r.Verdict != VerdictWorn {
		t.Fatalf("combined: %+v", r)
	}
}

func TestParseUnreadableBatteryIsUnknown(t *testing.T) {
	r, err := Parse(laptop(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Present || r.Verdict != VerdictUnknown || r.HealthPct != 0 || r.Batteries[0].Verdict != VerdictUnknown {
		t.Fatalf("a battery without a full-charge reading must be unknown: %+v", r)
	}
}

func TestParseEncodings(t *testing.T) {
	want, _ := Parse(laptop(48200, 312))
	bom := append([]byte("\xef\xbb\xbf"), laptop(48200, 312)...)
	if r, err := Parse(bom); err != nil || r.FullMWh != want.FullMWh {
		t.Fatalf("utf-8 bom: %v %+v", err, r)
	}
	u := utf16.Encode([]rune(string(laptop(48200, 312))))
	b := []byte{0xff, 0xfe}
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	if r, err := Parse(b); err != nil || r.FullMWh != want.FullMWh {
		t.Fatalf("utf-16: %v %+v", err, r)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "   ", "not xml at all", "<BatteryReport><Batteries>"} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%q should be an error", in)
		}
	}
}

func TestReadWithDeletesTheTemporaryFile(t *testing.T) {
	var seen string
	r, err := ReadWith(context.Background(), func(_ context.Context, path string) error {
		seen = path
		return os.WriteFile(path, laptop(48200, 312), 0o600)
	})
	if err != nil || !r.Present {
		t.Fatalf("%v %+v", err, r)
	}
	if _, err := os.Stat(seen); !os.IsNotExist(err) {
		t.Fatalf("temporary report left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(seen)); !os.IsNotExist(err) {
		t.Fatal("temporary folder left behind")
	}
}

func TestReadWithFailureAlsoCleansUp(t *testing.T) {
	var seen string
	_, err := ReadWith(context.Background(), func(_ context.Context, path string) error {
		seen = path
		_ = os.WriteFile(path, []byte("x"), 0o600)
		return fmt.Errorf("powercfg failed")
	})
	if err == nil {
		t.Fatal("want the error")
	}
	if _, err := os.Stat(filepath.Dir(seen)); !os.IsNotExist(err) {
		t.Fatal("temporary folder left behind after failure")
	}
	// A run that writes nothing is an error too, not an empty battery.
	if _, err := ReadWith(context.Background(), func(context.Context, string) error { return nil }); err == nil {
		t.Fatal("a missing report must be an error")
	}
}
