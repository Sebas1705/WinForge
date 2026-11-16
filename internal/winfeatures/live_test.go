//go:build windows

package winfeatures

import (
	"os"
	"testing"
)

// TestLiveReadOnly reads (never writes) this PC's real feature states and WSL distributions. It only
// runs when WINFORGE_LIVE=1, so CI and normal runs never touch the machine.
func TestLiveReadOnly(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read the real Windows features and WSL")
	}
	sys := NewSystem()
	for _, f := range List(sys, false) {
		t.Logf("%-8s state=%-11s restartPending=%v needsAdmin=%v", f.ID, f.State, f.RestartPending, f.NeedsAdmin)
		if f.State == Unknown {
			t.Errorf("%s could not be read without administrator rights", f.ID)
		}
	}
	info := WSL(sys)
	t.Logf("wsl installed=%v defaultVersion=%d distros=%d", info.Installed, info.DefaultVersion, len(info.Distros))
	for _, d := range info.Distros {
		t.Logf("  version=%d state=%s default=%v nameLen=%d", d.Version, d.State, d.IsDefault, len(d.Name))
	}
}
