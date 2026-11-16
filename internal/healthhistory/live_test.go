//go:build windows

package healthhistory

import (
	"context"
	"os"
	"testing"

	"github.com/Sebas1705/WinForge/internal/battery"
)

// TestLiveBatteryAndDrives reads (never writes anything that stays) this PC's battery report and
// drive counters. It only runs when WINFORGE_LIVE=1 and logs counts, never values.
func TestLiveBatteryAndDrives(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real battery report and drive counters")
	}
	b, err := battery.Read(context.Background())
	if err != nil {
		t.Fatalf("battery report: %v", err)
	}
	t.Logf("battery present=%v batteries=%d history=%d verdict=%s", b.Present, len(b.Batteries), len(b.History), b.Verdict)
	d, err := ReadDisks(context.Background())
	if err != nil {
		t.Fatalf("drive counters: %v", err)
	}
	withWear := 0
	for _, s := range d {
		if s.WearPct != nil {
			withWear++
		}
	}
	t.Logf("drives with counters=%d, reporting wear=%d (counters need administrator rights)", len(d), withWear)
}
