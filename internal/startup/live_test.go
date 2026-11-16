//go:build windows

package startup

import (
	"os"
	"testing"
)

// TestLiveList reads (never writes) this PC's real startup entries. It only runs when
// WINFORGE_LIVE=1, so CI and normal runs never touch the registry.
func TestLiveList(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real startup entries")
	}
	m := &Manager{Sys: NewSystem()}
	items, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		t.Logf("%-14s %-32q on=%v admin=%v missing=%v caution=%v pub=%q target=%q", it.Source, it.Name, it.Enabled, it.NeedsAdmin, it.Missing, it.Caution, it.Publisher, it.Target)
	}
	t.Logf("%d items", len(items))
}
