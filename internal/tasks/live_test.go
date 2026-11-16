//go:build windows

package tasks

import (
	"os"
	"testing"
)

// TestLiveList reads (never writes) this PC's real scheduled tasks and logs counts only. It runs
// only when WINFORGE_LIVE=1, so CI and normal runs never touch the machine.
func TestLiveList(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real scheduled tasks")
	}
	m := &Manager{Sys: NewSystem()}
	raws, err := m.Sys.List()
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	var logon, boot, disabled, perUser, withResult, caution int
	for _, it := range items {
		for _, tr := range it.Triggers {
			if tr == TriggerLogon {
				logon++
			}
			if tr == TriggerBoot {
				boot++
			}
		}
		if !it.Enabled {
			disabled++
		}
		if !it.NeedsAdmin {
			perUser++
		}
		if it.LastResult != nil {
			withResult++
		}
		if it.Caution {
			caution++
		}
	}
	t.Logf("%d tasks read outside \\Microsoft\\, %d shown (%d with a logon trigger, %d with a boot trigger, %d disabled, %d per-user, %d with a last result, %d caution)",
		len(raws), len(items), logon, boot, disabled, perUser, withResult, caution)
}
