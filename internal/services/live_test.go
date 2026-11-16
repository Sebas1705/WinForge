//go:build windows

package services

import (
	"os"
	"testing"
)

// TestLiveList reads (never writes) this PC's real services and logs counts only. It runs only
// when WINFORGE_LIVE=1, so CI and normal runs never touch the machine.
func TestLiveList(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real services")
	}
	sys := NewSystem()
	raws, err := sys.List()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Sys: sys}
	items, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	reasons := map[string]int{}
	env := Env{WinDir: os.Getenv("SystemRoot"), Lookup: os.Getenv}
	for _, r := range raws {
		reasons[Classify(r, env, "").Reason]++
	}
	var running, caution, withDeps, missingPub int
	for _, it := range items {
		if it.State == Running {
			running++
		}
		if it.Caution {
			caution++
		}
		if len(it.Dependents) > 0 {
			withDeps++
		}
		if it.Publisher == "" {
			missingPub++
		}
	}
	t.Logf("%d entries, %d shown (%d running, %d caution, %d with dependents, %d without publisher); hidden by reason before publisher check: %v",
		len(raws), len(items), running, caution, withDeps, missingPub, reasons)
}
