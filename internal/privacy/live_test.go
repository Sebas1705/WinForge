//go:build windows

package privacy

import (
	"os"
	"testing"
)

// TestLiveList reads (never writes) this PC's real registry. It only runs when WINFORGE_LIVE=1, so
// CI and normal runs never touch the registry. It logs counts, not values.
func TestLiveList(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real registry")
	}
	m := &Manager{Sys: NewSystem()}
	list, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range list {
		counts[v.State]++
		for _, c := range v.Changes {
			if c.Error {
				t.Errorf("%s: %s could not be read", v.ID, c.Name)
			}
		}
	}
	t.Logf("%d tweaks: %d private, %d default, %d custom, %d unknown",
		len(list), counts[StatePrivate], counts[StateDefault], counts[StateCustom], counts[StateUnknown])
	if b, err := m.Backups(); err == nil {
		t.Logf("%d backups on disk", len(b))
	}
}
