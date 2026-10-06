//go:build live

package health_test

import (
	"context"
	"testing"

	"github.com/Sebas1705/WinForge/internal/health"
)

// Run on a real Windows PC: go test -tags live -run Live -v ./internal/health
func TestLiveCollect(t *testing.T) {
	r, err := health.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s %s | %s | board %s %s | BIOS %s %s %s", r.Machine.Manufacturer, r.Machine.Model, r.OS.Caption, r.Board.Manufacturer, r.Board.Product, r.BIOS.Vendor, r.BIOS.Version, r.BIOS.Date)
	t.Logf("cpu %s %d/%d | ram %.1f GB in %d modules | admin %v uefi %v", r.CPU.Name, r.CPU.Cores, r.CPU.Threads, r.Memory.TotalGB, len(r.Memory.Modules), r.Admin, r.BIOS.UEFI)
	for _, g := range r.GPUs {
		t.Logf("gpu %s [%s] %s %s", g.Name, g.Vendor, g.DriverVersion, g.DriverDate)
	}
	t.Logf("%d drivers, %d problem devices, %d disks, %d volumes, errors: %v", len(r.Drivers), len(r.Problems), len(r.Disks), len(r.Volumes), r.Errors)
	t.Logf("security: av=%+v defender=%+v fw=%+v secureBoot=%v tpm=%+v", r.Security.AntiVirus, r.Security.Defender, r.Security.Firewall, r.Security.SecureBoot, r.Security.TPM)
	for _, p := range r.Problems {
		t.Logf("problem device: %q [%s] %s code %d", p.Device, p.Class, p.HardwareID, p.Code)
	}
	old := 0
	for _, d := range r.Drivers {
		if d.Date != "" && d.Date < "2023-01-01" && d.Manufacturer != "Microsoft" {
			old++
			if old <= 8 {
				t.Logf("old third-party driver: %s | %s | %s | %s %s", d.Device, d.Class, d.Manufacturer, d.Version, d.Date)
			}
		}
	}
	t.Logf("%d third-party drivers older than 2023", old)
	if len(r.Drivers) == 0 {
		t.Fatal("no drivers read")
	}
}
