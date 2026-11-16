//go:build windows

package sched

import (
	"os"
	"testing"
)

// TestLiveQuery reads (never writes) whether this PC has the "WinForge updates" task. It only runs when
// WINFORGE_LIVE=1, so CI and normal runs never touch the scheduler.
func TestLiveQuery(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to ask the real Task Scheduler")
	}
	info, err := NewSystem().Query()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("task exists=%v enabled=%v foreign=%v settings=%+v exe=%q args=%q", info.Exists, info.Enabled, info.Foreign, info.Settings, info.Exe, info.Args)
	t.Logf("system language: %q", SystemLanguage())
}
