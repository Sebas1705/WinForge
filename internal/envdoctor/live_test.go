//go:build windows

package envdoctor

import (
	"os"
	"testing"
)

// TestLiveScan reads (never writes) this PC's real environment. It only runs when WINFORGE_LIVE=1,
// so CI and normal runs never touch the registry. It logs counts and kinds, not folder names.
func TestLiveScan(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to scan the real environment")
	}
	m := &Manager{Sys: NewSystem()}
	r, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	fixable := 0
	for _, f := range r.Findings {
		counts[f.Kind+"/"+string(f.Scope)]++
		if f.Fixable {
			fixable++
		}
	}
	t.Logf("user PATH %d entries, machine PATH %d entries, %d variables, %d findings (%d fixable)",
		len(r.UserPath), len(r.MachinePath), len(r.Vars), len(r.Findings), fixable)
	for k, n := range counts {
		t.Logf("  %-32s %d", k, n)
	}
	for _, tl := range r.Tools {
		if len(tl.Found) > 0 {
			t.Logf("  tool %-8s %d copies on PATH", tl.Name, len(tl.Found))
		}
	}
	if b, err := m.Backups(); err == nil {
		t.Logf("%d backups on disk", len(b))
	}
}
