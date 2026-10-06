package health_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/health"
)

var now = time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

func yes(b bool) *bool { return &b }

func find(t *testing.T, fs []health.Finding, key string) health.Finding {
	t.Helper()
	for _, f := range fs {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("no finding %q in %v", key, keysOf(fs))
	return health.Finding{}
}

func keysOf(fs []health.Finding) []string {
	var k []string
	for _, f := range fs {
		k = append(k, f.Key+"="+string(f.Severity))
	}
	return k
}

// healthy is a PC where every check passes.
func healthy() *health.Report {
	return &health.Report{
		Machine: health.Machine{Manufacturer: "ASUS", Model: "System Product Name"},
		Board:   health.Board{Manufacturer: "ASUSTeK COMPUTER INC.", Product: "ROG STRIX X870-A GAMING WIFI"},
		BIOS:    health.BIOS{Vendor: "AMI", Version: "2402", Date: "2026-07-13", UEFI: yes(true)},
		OS:      health.OS{Caption: "Microsoft Windows 11 Pro", UptimeDays: 3},
		CPU:     health.CPU{VirtualizationOn: yes(true)},
		GPUs:    []health.GPU{{Name: "NVIDIA GeForce RTX 4080 SUPER", Vendor: "NVIDIA", DriverVersion: "32.0.16.1714", DriverDate: "2026-09-17"}},
		Disks:   []health.Disk{{Name: "Samsung 990", Health: "Healthy"}},
		Volumes: []health.Volume{{Letter: "C", SizeGB: 1000, FreeGB: 400}},
		Security: health.Security{
			AntiVirus:              []health.AV{{Name: "Windows Defender", Enabled: true, UpToDate: true}},
			Defender:               &health.Defender{Enabled: true, RealTime: true, SignatureAgeDays: 0.5},
			Firewall:               []health.Firewall{{Profile: "Public", Enabled: true}},
			SecureBoot:             yes(true),
			TPM:                    &health.TPM{Present: true, Ready: true},
			BitLockerOnSystemDrive: yes(true),
		},
		Drivers: []health.Driver{
			{Device: "NVIDIA GeForce RTX 4080 SUPER", Class: "DISPLAY", Manufacturer: "NVIDIA", Version: "32.0.16.1714", Date: "2026-09-17", Signed: true},
			{Device: "Realtek Audio", Class: "MEDIA", Manufacturer: "Realtek", Version: "6.0.9700.1", Date: "2025-11-01", Signed: true},
		},
	}
}

func TestHealthyPCPassesEveryCheck(t *testing.T) {
	r := healthy()
	fs := health.Analyze(r, &health.UpdateScan{}, now)
	for _, f := range fs {
		if f.Severity != health.OK {
			t.Errorf("%s should pass, got %s", f.Key, f.Severity)
		}
	}
	if ok, total := health.Tally(fs); ok != total || total < 15 {
		t.Fatalf("tally %d/%d", ok, total)
	}
}

func TestEveryKeyIsEmitted(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range health.Analyze(healthy(), &health.UpdateScan{}, now) {
		seen[f.Key] = true
	}
	for _, k := range health.Keys {
		if !seen[k] {
			t.Errorf("Keys lists %q but Analyze never emits it", k)
		}
	}
	for k := range seen {
		found := false
		for _, kk := range health.Keys {
			found = found || kk == k
		}
		if !found {
			t.Errorf("Analyze emits %q which is missing from Keys", k)
		}
	}
}

func TestBIOSAgeThresholds(t *testing.T) {
	cases := map[string]health.Severity{"2026-07-13": health.OK, "2025-04-01": health.Info, "2023-01-01": health.Warn, "": health.Unknown}
	for date, want := range cases {
		r := healthy()
		r.BIOS.Date = date
		f := find(t, health.Analyze(r, nil, now), "bios-age")
		if f.Severity != want {
			t.Errorf("BIOS %q: got %s want %s", date, f.Severity, want)
		}
	}
}

func TestBIOSLinksUseBoardWhenSystemNameIsPlaceholder(t *testing.T) {
	f := find(t, health.Analyze(healthy(), nil, now), "bios-age")
	if len(f.Links) != 2 || f.Links[0].Label != "ASUS" || !strings.Contains(f.Links[1].URL, "ROG+STRIX+X870-A") {
		t.Fatalf("%+v", f.Links)
	}
	if f.Params["board"] != "ASUSTeK COMPUTER INC. ROG STRIX X870-A GAMING WIFI" {
		t.Fatal(f.Params["board"])
	}
	// A real OEM laptop is identified by its system model, not the board.
	r := healthy()
	r.Machine = health.Machine{Manufacturer: "LENOVO", Model: "ThinkPad X1 Carbon Gen 11"}
	f = find(t, health.Analyze(r, nil, now), "bios-age")
	if f.Links[0].Label != "Lenovo" || !strings.Contains(f.Links[1].URL, "ThinkPad+X1") {
		t.Fatalf("%+v", f.Links)
	}
}

func TestDeviceProblemsNameTheMissingDriver(t *testing.T) {
	r := healthy()
	r.Problems = []health.Problem{{HardwareID: `ACPI\RTK5452\1`, Code: 28}, {Device: "Unknown device", HardwareID: `PCI\VEN_8086`, Code: 28}}
	f := find(t, health.Analyze(r, nil, now), "device-problems")
	if f.Severity != health.Bad || f.Params["count"] != "2" || !strings.Contains(f.Params["devices"], `Realtek ACPI\RTK5452\1`) || !strings.Contains(f.Params["devices"], "Unknown device") {
		t.Fatalf("%+v", f)
	}
	if len(f.Links) == 0 || f.Links[0].Label != "ASUS" {
		t.Fatal("should point to the board vendor's support page")
	}
}

func TestInBoxWindowsDriversAreNeverCalledOld(t *testing.T) {
	r := healthy()
	r.Drivers = append(r.Drivers,
		health.Driver{Device: "ACPI Fixed Feature Button", Class: "SYSTEM", Manufacturer: "(Standard system devices)", Version: "10.0.26100.1150", Date: "2006-06-21", Signed: true},
		health.Driver{Device: "AMD Processor", Class: "PROCESSOR", Manufacturer: "Advanced Micro Devices", Version: "10.0.26100.9278", Date: "2009-04-21", Signed: true},
		health.Driver{Device: "Microsoft Basic Thing", Class: "SYSTEM", Manufacturer: "Microsoft", Version: "1.0", Date: "2010-01-01", Signed: true})
	if f := find(t, health.Analyze(r, nil, now), "drivers-old"); f.Severity != health.OK {
		t.Fatalf("in-box drivers flagged: %+v", f)
	}
	r.Drivers = append(r.Drivers,
		health.Driver{Device: "Old NIC", Class: "NET", Manufacturer: "Realtek", Version: "10.1.2.3", Date: "2020-02-02", Signed: true},
		health.Driver{Device: "Old mouse", Class: "MOUSE", Manufacturer: "Acme", Version: "1.0", Date: "2015-02-02", Signed: true})
	f := find(t, health.Analyze(r, nil, now), "drivers-old")
	if f.Severity != health.Warn || f.Params["count"] != "1" || !strings.Contains(f.Params["devices"], "Old NIC (2020-02-02)") {
		t.Fatalf("only important classes count: %+v", f)
	}
}

func TestUnsignedDrivers(t *testing.T) {
	r := healthy()
	r.Drivers = append(r.Drivers, health.Driver{Device: "Shady", Class: "SYSTEM", Manufacturer: "Who", Version: "1.0", Date: "2024-01-01", Signed: false})
	if f := find(t, health.Analyze(r, nil, now), "drivers-unsigned"); f.Severity != health.Warn {
		t.Fatalf("%+v", f)
	}
}

func TestGPUDriverFreshnessAndFriendlyVersion(t *testing.T) {
	g := health.GPU{Vendor: "NVIDIA", DriverVersion: "32.0.15.6094"}
	if got := health.FriendlyGPUVersion(g); got != "560.94" {
		t.Fatal(got)
	}
	if got := health.FriendlyGPUVersion(health.GPU{Vendor: "NVIDIA", DriverVersion: "32.0.16.1714"}); got != "617.14" {
		t.Fatal(got)
	}
	if got := health.FriendlyGPUVersion(health.GPU{Vendor: "AMD", DriverVersion: "32.0.21042.62"}); got != "32.0.21042.62" {
		t.Fatal(got)
	}
	r := healthy()
	r.GPUs[0].DriverDate = "2026-01-01"
	f := find(t, health.Analyze(r, nil, now), "gpu-driver")
	if f.Severity != health.Info || f.Links[0].Label != "NVIDIA" {
		t.Fatalf("%+v", f)
	}
	r.GPUs = []health.GPU{{Name: "Intel(R) UHD Graphics", Vendor: "Intel", DriverVersion: "31.0.101.5186", DriverDate: "2026-09-01"}, {Name: "Microsoft Basic Display Adapter", Vendor: "Microsoft"}}
	fs := health.Analyze(r, nil, now)
	n := 0
	for _, f := range fs {
		if f.Key == "gpu-driver" {
			n++
			if f.Winget != "Intel.IntelDriverAndSupportAssistant" {
				t.Fatalf("Intel GPUs should recommend Intel DSA: %+v", f)
			}
		}
	}
	if n != 1 {
		t.Fatalf("the basic display adapter must be skipped, got %d GPU findings", n)
	}
}

func TestStorageChecks(t *testing.T) {
	r := healthy()
	r.Disks[0].Health = "Unhealthy"
	r.Volumes = []health.Volume{{Letter: "C", SizeGB: 500, FreeGB: 200}, {Letter: "D", SizeGB: 1000, FreeGB: 300}, {Letter: "E", SizeGB: 100, FreeGB: 15}}
	fs := health.Analyze(r, nil, now)
	if f := find(t, fs, "disk-health"); f.Severity != health.Bad || !strings.Contains(f.Params["disks"], "Samsung 990 (Unhealthy)") {
		t.Fatalf("%+v", f)
	}
	f := find(t, fs, "disk-space")
	if f.Severity != health.Warn || strings.Contains(f.Params["volumes"], "C:") || strings.Contains(f.Params["volumes"], "D:") || !strings.Contains(f.Params["volumes"], "E:") {
		t.Fatalf("only E (15%% but under 20 GB) is low: %+v", f)
	}
}

func TestSecurityChecks(t *testing.T) {
	r := healthy()
	r.Security.AntiVirus = nil
	r.Security.Defender = &health.Defender{Enabled: false}
	if f := find(t, health.Analyze(r, nil, now), "antivirus"); f.Severity != health.Bad {
		t.Fatalf("no active antivirus must be bad: %+v", f)
	}
	r = healthy()
	r.Security.Defender.SignatureAgeDays = 12
	if f := find(t, health.Analyze(r, nil, now), "av-signatures"); f.Severity != health.Warn {
		t.Fatalf("%+v", f)
	}
	r = healthy()
	r.Security.Firewall = []health.Firewall{{Profile: "Public", Enabled: false}, {Profile: "Private", Enabled: true}}
	if f := find(t, health.Analyze(r, nil, now), "firewall"); f.Severity != health.Warn || f.Params["profiles"] != "Public" {
		t.Fatalf("%+v", f)
	}
	r = healthy()
	r.Security.BitLockerOnSystemDrive = nil
	r.Security.SecureBoot = nil
	fs := health.Analyze(r, nil, now)
	if find(t, fs, "bitlocker").Severity != health.Unknown || find(t, fs, "secure-boot").Severity != health.Unknown {
		t.Fatal("checks that need administrator rights are unknown, not failed")
	}
	if ok, total := health.Tally(fs); total != len(fs)-2 || ok != total {
		t.Fatalf("unknown results must not count against the PC: %d/%d of %d", ok, total, len(fs))
	}
}

func TestWindowsChecks(t *testing.T) {
	r := healthy()
	r.OS.Caption = "Microsoft Windows 10 Pro"
	if f := find(t, health.Analyze(r, nil, now), "windows-eol"); f.Severity != health.Bad {
		t.Fatalf("Windows 10 is out of support: %+v", f)
	}
	if f := find(t, health.Analyze(r, nil, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)), "windows-eol"); f.Severity != health.OK {
		t.Fatal("still supported before 14 Oct 2025")
	}
	r = healthy()
	r.OS.PendingReboot = true
	r.OS.UptimeDays = 45
	fs := health.Analyze(r, nil, now)
	if find(t, fs, "pending-reboot").Severity != health.Warn || find(t, fs, "uptime").Severity != health.Info {
		t.Fatal(keysOf(fs))
	}
}

func TestWindowsUpdateFindings(t *testing.T) {
	scan := &health.UpdateScan{Updates: []health.Update{
		{Title: "Realtek - Audio", Kind: "Driver", Category: "Drivers"},
		{Title: "AMD - Chipset", Kind: "Driver", Category: "Drivers"},
		{Title: "2026-10 Cumulative", Kind: "Software"},
	}}
	fs := health.Analyze(healthy(), scan, now)
	if f := find(t, fs, "wu-drivers"); f.Severity != health.Warn || f.Params["count"] != "2" {
		t.Fatalf("%+v", f)
	}
	if f := find(t, fs, "wu-software"); f.Severity != health.Info || f.Params["count"] != "1" {
		t.Fatalf("%+v", f)
	}
	for _, f := range health.Analyze(healthy(), nil, now) {
		if strings.HasPrefix(f.Key, "wu-") {
			t.Fatal("update findings only appear after a Windows Update scan")
		}
	}
}

func TestVendorLinks(t *testing.T) {
	for in, want := range map[string]string{"ASUSTeK COMPUTER INC.": "ASUS", "Micro-Star International Co., Ltd.": "MSI", "Gigabyte Technology Co., Ltd.": "Gigabyte",
		"Dell Inc.": "Dell", "HP": "", "Hewlett-Packard": "HP", "LENOVO": "Lenovo", "System manufacturer": ""} {
		l, ok := health.SupportLink(in)
		if (want == "") == ok || (ok && l.Label != want) {
			t.Errorf("%q: got %q ok=%v want %q", in, l.Label, ok, want)
		}
	}
	for id, want := range map[string]string{`ACPI\AMDI0204\2&DABA3FF&0`: "AMD", `ACPI\RTK5452\1`: "Realtek", `PCI\VEN_8086&DEV_1`: "Intel", `USB\VID_0000`: ""} {
		if got := health.HardwareHint(id); got != want {
			t.Errorf("%s: %q want %q", id, got, want)
		}
	}
}

func TestParseReportToleratesNoiseAndNormalizes(t *testing.T) {
	raw := "\xef\xbb\xbfWARNING: something noisy\n" + `{"admin":true,"machine":{"manufacturer":"ASUS","model":"X"},"os":{"caption":"Windows 11"},
	"gpus":[{"name":"AMD Radeon(TM) Graphics","vendor":"Advanced Micro Devices, Inc.","driverVersion":"1","driverDate":"2026-04-17"}],
	"bios":{"version":"2402","date":"2026-07-13","uefi":true},"drivers":null,"errors":[]}`
	r, err := health.ParseReport([]byte(raw), now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Admin || r.GPUs[0].Vendor != "AMD" || r.Drivers == nil || r.Disks == nil || r.Problems == nil || r.Security.AntiVirus == nil || r.CollectedAt != now {
		t.Fatalf("%+v", r)
	}
	if _, err := health.ParseReport([]byte("not json"), now); err == nil {
		t.Fatal("garbage must be an error")
	}
	s, err := health.ParseUpdates([]byte(`{"updates":[{"title":"x","kind":"Driver","sizeMB":1.5,"reboot":true}]}`), now)
	if err != nil || len(s.Updates) != 1 || s.Updates[0].Kind != "Driver" {
		t.Fatalf("%+v %v", s, err)
	}
	if s, _ := health.ParseUpdates([]byte(`{"updates":null}`), now); s.Updates == nil {
		t.Fatal("no updates must be an empty list")
	}
}

func TestTPMNegativeIsUnknownWithoutAdmin(t *testing.T) {
	r := healthy()
	r.Security.TPM = &health.TPM{Present: false}
	r.Admin = false
	if f := find(t, health.Analyze(r, nil, now), "tpm"); f.Severity != health.Unknown {
		t.Fatalf("%+v", f)
	}
	r.Admin = true
	if f := find(t, health.Analyze(r, nil, now), "tpm"); f.Severity != health.Info {
		t.Fatalf("an elevated scan with no TPM is a real finding: %+v", f)
	}
}
