package services

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Sebas1705/WinForge/internal/origstate"
)

var testEnv = Env{WinDir: `C:\Windows`, Lookup: NewFake().Getenv}

const (
	own    = 0x10
	shared = 0x20
)

func svc(name, display string, typ, start uint32, image string, deps ...string) Raw {
	return Raw{Name: name, DisplayName: display, Type: typ, Start: start, ImagePath: image, DependsOn: deps}
}

func TestClassifyTable(t *testing.T) {
	type c struct {
		name      string
		raw       Raw
		publisher string
		show      bool
		caution   bool
		reason    string
	}
	cases := []c{
		// Third-party services that must be shown.
		{"steam client", svc("Steam Client Service", "Steam Client Service", own, 3, `"C:\Program Files (x86)\Steam\bin\steamservice.exe"`), "Valve Corporation", true, false, ReasonShown},
		{"nvidia display container", svc("NVDisplay.ContainerLocalSystem", "NVIDIA Display Container LS", own, 2, `"C:\Windows\System32\DriverStore\FileRepository\nv_dispi.inf_amd64_x\Display.NvContainer\NVDisplay.Container.exe" -s NVDisplayContainerLocalSystem`), "NVIDIA", false, false, ReasonWindows},
		{"docker", svc("com.docker.service", "Docker Desktop Service", own, 3, `"C:\Program Files\Docker\Docker\com.docker.service"`), "Docker Inc.", true, false, ReasonShown},
		{"unquoted path with args", svc("MyApp", "My App", own, 2, `C:\Program Files\MyApp\myapp.exe -service`), "", true, false, ReasonShown},
		{"env variable path", svc("VendorSvc", "Vendor", own, 2, `%ProgramFiles%\Vendor\svc.exe`), "Vendor Ltd", true, false, ReasonShown},
		{"delayed auto", Raw{Name: "OneSync", DisplayName: "OneSync", Type: own, Start: 2, Delayed: true, ImagePath: `"D:\Apps\OneSync\sync.exe"`}, "Acme", true, false, ReasonShown},
		{"disabled third party", svc("OldAgent", "Old Agent", own, 4, `"C:\Program Files\Old\agent.exe"`), "Old Inc", true, false, ReasonShown},
		{"shared process third party", svc("VendorHost", "Vendor Host", shared, 3, `"C:\Program Files\Vendor\host.exe" /svc`), "Vendor", true, false, ReasonShown},
		{"no publisher known", svc("Mystery", "Mystery Service", own, 3, `"C:\Tools\mystery.exe"`), "", true, false, ReasonShown},
		{"google updater", svc("gupdate", "Google Update Service (gupdate)", own, 2, `"C:\Program Files (x86)\Google\Update\GoogleUpdate.exe" /svc`), "Google LLC", true, false, ReasonShown},

		// Security products: shown, but ask first.
		{"kaspersky", svc("AVP21.3", "Kaspersky Service", own, 2, `"C:\Program Files (x86)\Kaspersky Lab\Kaspersky 21.3\avp.exe" -r`), "AO Kaspersky Lab", true, true, ReasonShown},
		{"malwarebytes", svc("MBAMService", "Malwarebytes Service", own, 2, `"C:\Program Files\Malwarebytes\Anti-Malware\mbamservice.exe"`), "Malwarebytes", true, true, ReasonShown},
		{"bitdefender", svc("VSSERV", "Bitdefender Virus Shield", own, 2, `"C:\Program Files\Bitdefender\Bitdefender Security\vsserv.exe"`), "Bitdefender", true, true, ReasonShown},
		{"avast by publisher", svc("avast", "Avast Service", own, 2, `"C:\Program Files\Avast Software\Avast\AvastSvc.exe"`), "AVAST Software s.r.o.", true, true, ReasonShown},
		{"crowdstrike", svc("CSFalconService", "CrowdStrike Falcon Sensor Service", own, 2, `"C:\Program Files\CrowdStrike\CSFalconService.exe"`), "CrowdStrike, Inc.", true, true, ReasonShown},
		{"third-party firewall", svc("GlassWireSvc", "Network Firewall Helper", own, 2, `"C:\Program Files\Acme\fw.exe"`), "Acme", true, true, ReasonShown},
		{"eset", svc("ekrn", "ESET Service", own, 2, `"C:\Program Files\ESET\ESET Security\ekrn.exe"`), "ESET, spol. s r.o.", true, true, ReasonShown},

		// Near-misses of the security words must stay plain.
		{"navigation is not avg", svc("NavSvc", "Navigation Service", own, 3, `"C:\Program Files\Nav\nav.exe"`), "Nav Co", true, false, ReasonShown},
		{"tendency is not trend", svc("Tendency", "Tendency Sync", own, 3, `"C:\Program Files\Tendency\t.exe"`), "T Co", true, false, ReasonShown},

		// Microsoft and Windows: hidden.
		{"svchost hosted", svc("Dhcp", "DHCP Client", shared, 2, `C:\Windows\system32\svchost.exe -k LocalServiceNetworkRestricted -p`), "Microsoft Corporation", false, false, ReasonProtected},
		{"svchost, unprotected name", svc("FontCache", "Windows Font Cache Service", shared, 2, `C:\Windows\system32\svchost.exe -k LocalService -p`), "", false, false, ReasonWindows},
		{"SystemRoot form", svc("Foo", "Foo", own, 2, `\SystemRoot\System32\foo.exe`), "", false, false, ReasonWindows},
		{"percent SystemRoot", svc("Foo", "Foo", own, 2, `%SystemRoot%\system32\foo.exe`), "", false, false, ReasonWindows},
		{"upper-case windir", svc("Foo", "Foo", own, 2, `"C:\WINDOWS\SysWOW64\foo.exe"`), "", false, false, ReasonWindows},
		{"forward slashes under windir", svc("Foo", "Foo", own, 2, `C:/Windows/System32/foo.exe`), "", false, false, ReasonWindows},
		{"windows.old folder is hidden too (unsure)", svc("WindowsBackup2", "Backup", own, 3, `"C:\Windows.old\tool\b.exe"`), "Vendor", false, false, ReasonWindows}, // folder starting with "windows"
		{"sql server (Microsoft folder)", svc("MSSQLSERVER", "SQL Server (MSSQLSERVER)", own, 2, `"C:\Program Files\Microsoft SQL Server\MSSQL16.MSSQLSERVER\MSSQL\Binn\sqlservr.exe" -sMSSQLSERVER`), "", false, false, ReasonWindows},
		{"defender platform in ProgramData", svc("MsMpEng2", "Defender Antivirus", own, 2, `"C:\ProgramData\Microsoft\Windows Defender\Platform\4.18.2309.6-0\MsMpEng.exe"`), "Microsoft Corporation", false, false, ReasonProtected},
		{"microsoft publisher outside windows", svc("OfficeSvc", "Office Source Engine", own, 3, `"C:\Program Files\Common Files\Acme\ose.exe"`), "Microsoft Corporation", false, false, ReasonMicrosoft},
		{"microsoft publisher mixed case", svc("Svc2", "Svc2", own, 3, `"D:\x\y.exe"`), "MICROSOFT CORP.", false, false, ReasonMicrosoft},
		{"claims Microsoft without publisher", svc("MicrosoftThing", "Microsoft Thing Helper", own, 3, `"D:\x\y.exe"`), "", false, false, ReasonMicrosoft},
		{"windows apps", svc("StoreSvc", "Store", own, 3, `"C:\Program Files\WindowsApps\Foo_1.0\foo.exe"`), "", false, false, ReasonWindows},

		// Drivers and kernel-level entries.
		{"kernel driver", svc("npcap", "Npcap Packet Driver", 0x1, 1, `\SystemRoot\system32\DRIVERS\npcap.sys`), "Insecure.Com LLC", false, false, ReasonDriver},
		{"file system driver", svc("VBoxSF", "VirtualBox Shared Folders", 0x2, 3, `system32\DRIVERS\VBoxSF.sys`), "Oracle", false, false, ReasonDriver},
		{"third-party driver outside windows", svc("MyDrv", "My Driver", 0x1, 3, `\??\C:\Program Files\My\my.sys`), "My", false, false, ReasonDriver},
		{"driver-ish service with boot start", svc("Boot", "Boot", own, 0, `C:\x\y.exe`), "", false, false, ReasonDriver},
		{"adapter", svc("Ad", "Ad", 0x4, 3, `C:\x\y.sys`), "", false, false, ReasonDriver},
		{"recognizer", svc("Rec", "Rec", 0x8, 3, `C:\x\y.sys`), "", false, false, ReasonDriver},

		// Per-user template services and odd types.
		{"user service template", svc("OneSyncSvc", "Sync Host", 0x60, 2, `C:\Program Files\X\x.exe`), "X", false, false, ReasonUserService},
		{"user own process", svc("CDPUserSvc_3a4b5", "CDP", 0x50, 2, `C:\Program Files\X\x.exe`), "X", false, false, ReasonUserService},
		{"no process type", svc("Weird", "Weird", 0x0, 3, `C:\Program Files\X\x.exe`), "X", false, false, ReasonNotService},

		// Protected list, even with a vendor binary (near-misses by name).
		{"WinDefend", svc("WinDefend", "Microsoft Defender Antivirus Service", own, 2, `"C:\ProgramData\Microsoft\Windows Defender\Platform\x\MsMpEng.exe"`), "", false, false, ReasonProtected},
		{"Sense", svc("Sense", "Defender for Endpoint", own, 3, `"C:\Program Files\Windows Defender Advanced Threat Protection\MsSense.exe"`), "", false, false, ReasonProtected},
		{"wscsvc", svc("wscsvc", "Security Center", shared, 2, `C:\Program Files\Vendor\wsc.exe`), "Vendor", false, false, ReasonProtected},
		{"EventLog", svc("EventLog", "Windows Event Log", shared, 2, `C:\Program Files\Vendor\ev.exe`), "Vendor", false, false, ReasonProtected},
		{"Windows Update", svc("wuauserv", "Windows Update", shared, 3, `C:\Program Files\Vendor\wu.exe`), "Vendor", false, false, ReasonProtected},
		{"RpcSs", svc("RpcSs", "Remote Procedure Call (RPC)", shared, 2, `D:\r.exe`), "", false, false, ReasonProtected},
		{"DcomLaunch", svc("DcomLaunch", "DCOM Server Process Launcher", shared, 2, `D:\r.exe`), "", false, false, ReasonProtected},
		{"BITS", svc("BITS", "Background Intelligent Transfer", shared, 3, `D:\r.exe`), "", false, false, ReasonProtected},
		{"mixed case protected", svc("wInDeFeNd", "x", own, 2, `D:\r.exe`), "", false, false, ReasonProtected},
		{"protected prefix", svc("WdNisSvc", "x", own, 2, `D:\r.exe`), "", false, false, ReasonProtected},
		{"similar name is not protected", svc("BitsAndBytesSync", "Bits Sync", own, 2, `"D:\Apps\bits\sync.exe"`), "Acme", true, false, ReasonShown},

		// Cannot be judged: hidden, not shown.
		{"empty image path", svc("Empty", "Empty", own, 3, ``), "", false, false, ReasonNoBinary},
		{"relative path", svc("Rel", "Rel", own, 3, `tool.exe`), "", false, false, ReasonNoBinary},
		{"unresolved variable", svc("Var", "Var", own, 3, `%UNKNOWNVAR%\tool.exe`), "", false, false, ReasonNoBinary},
		{"launcher", svc("Cmd", "Cmd", own, 3, `cmd.exe /c something`), "", false, false, ReasonNoBinary},
		{"unknown start value", svc("S9", "S9", own, 7, `D:\a.exe`), "", false, false, ReasonStartMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.raw, testEnv, tc.publisher)
			if got.Show != tc.show || got.Caution != tc.caution || got.Reason != tc.reason {
				t.Fatalf("Classify(%s) = show %v caution %v reason %q; want show %v caution %v reason %q", tc.raw.Name, got.Show, got.Caution, got.Reason, tc.show, tc.caution, tc.reason)
			}
		})
	}
}

func TestClassifyHidesEverythingWhenWindirIsUnknown(t *testing.T) {
	got := Classify(svc("Steam", "Steam", own, 3, `"C:\Program Files\Steam\s.exe"`), Env{Lookup: NewFake().Getenv}, "Valve")
	if got.Show || got.Reason != ReasonUnsure {
		t.Fatalf("got %+v, want hidden as unsure", got)
	}
}

func TestExecutablePath(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\A\a.exe" -k x`:    `C:\Program Files\A\a.exe`,
		`C:\Program Files\A\a.exe -k x`:      `C:\Program Files\A\a.exe`,
		`\??\C:\Program Files\A\a.exe`:       `C:\Program Files\A\a.exe`,
		`\SystemRoot\System32\drivers\x.sys`: `C:\Windows\System32\drivers\x.sys`,
		`system32\DRIVERS\x.sys`:             `C:\Windows\system32\DRIVERS\x.sys`,
		`%ProgramFiles%\A\a.exe`:             `C:\Program Files\A\a.exe`,
		`"C:\Program Files (x86)\A B\a.exe"`: `C:\Program Files (x86)\A B\a.exe`,
		``:                                   ``,
		`a.exe`:                              ``,
		`rundll32.exe C:\x.dll,Start`:        ``,
	}
	for in, want := range cases {
		if got := ExecutablePath(in, testEnv); got != want {
			t.Errorf("ExecutablePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModeOf(t *testing.T) {
	cases := []struct {
		start   uint32
		delayed bool
		want    Mode
		ok      bool
	}{
		{2, false, Auto, true}, {2, true, Delayed, true}, {3, false, Manual, true}, {3, true, Manual, true},
		{4, false, Disabled, true}, {4, true, Disabled, true}, {0, false, "", false}, {1, false, "", false}, {5, false, "", false},
	}
	for _, c := range cases {
		got, ok := ModeOf(c.start, c.delayed)
		if got != c.want || ok != c.ok {
			t.Errorf("ModeOf(%d,%v) = %q,%v want %q,%v", c.start, c.delayed, got, ok, c.want, c.ok)
		}
	}
}

// --- manager -------------------------------------------------------------------------------

type rig struct {
	fake  *Fake
	m     *Manager
	admin bool
	store *origstate.Store
}

func newRig(t *testing.T) *rig {
	r := &rig{fake: NewFake(), admin: true, store: &origstate.Store{Path: filepath.Join(t.TempDir(), "services-original.json")}}
	r.fake.Services = []Raw{
		svc("Steam Client Service", "Steam Client Service", own, 3, `"C:\Program Files (x86)\Steam\bin\steamservice.exe"`),
		{Name: "OneSync", DisplayName: "Acme Sync", Type: own, Start: 2, Delayed: true, ImagePath: `"D:\Apps\sync.exe"`},
		svc("AVP21", "Kaspersky Service", own, 2, `"C:\Program Files\Kaspersky Lab\avp.exe"`),
		svc("Parent", "Vendor Parent", own, 2, `"C:\Program Files\Vendor\parent.exe"`),
		svc("Child", "Vendor Child", own, 3, `"C:\Program Files\Vendor\child.exe"`, "Parent"),
		svc("WinDefend", "Microsoft Defender Antivirus Service", own, 2, `C:\ProgramData\Microsoft\Windows Defender\MsMpEng.exe`),
		svc("Dhcp", "DHCP Client", shared, 2, `C:\Windows\system32\svchost.exe -k x`),
		svc("npcap", "Npcap", 0x1, 3, `\SystemRoot\system32\DRIVERS\npcap.sys`),
		svc("MsDependsOnParent", "Windows Thing", shared, 2, `C:\Windows\system32\svchost.exe -k y`, "parent"),
	}
	r.fake.Running["Steam Client Service"] = Running
	r.fake.Running["OneSync"] = Stopped
	r.fake.Companies[`C:\Program Files (x86)\Steam\bin\steamservice.exe`] = "Valve Corporation"
	r.m = &Manager{Sys: r.fake, Elevated: func() bool { return r.admin }, Store: r.store}
	return r
}

func ids(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestListShowsOnlyThirdPartyServicesWithTheirDetails(t *testing.T) {
	r := newRig(t)
	items, err := r.m.List()
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by display name; Defender, DHCP and the driver are hidden.
	if got, want := ids(items), []string{"OneSync", "AVP21", "Steam Client Service", "Child", "Parent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	s := byID["Steam Client Service"]
	if s.Publisher != "Valve Corporation" || s.State != Running || s.StartMode != Manual || !s.NeedsAdmin || s.Binary != `C:\Program Files (x86)\Steam\bin\steamservice.exe` {
		t.Fatalf("steam = %+v", s)
	}
	if o := byID["OneSync"]; o.StartMode != Delayed || o.State != Stopped || o.Changed {
		t.Fatalf("onesync = %+v", o)
	}
	if k := byID["AVP21"]; !k.Caution || k.State != Other {
		t.Fatalf("kaspersky = %+v (state of an unreadable service is other)", k)
	}
	if p := byID["Parent"]; !reflect.DeepEqual(p.Dependents, []string{"Vendor Child", "Windows Thing"}) {
		t.Fatalf("dependents = %v (case-insensitive names, hidden dependents count too)", p.Dependents)
	}
	if byID["Child"].Dependents == nil || len(byID["Child"].Dependents) != 0 {
		t.Fatal("no dependents must be an empty list, not null")
	}
}

func TestListFailsWhenTheMachineCannotBeRead(t *testing.T) {
	m := &Manager{Sys: failingSys{}}
	if _, err := m.List(); err == nil {
		t.Fatal("want the read error")
	}
}

type failingSys struct{ *Fake }

func (failingSys) List() ([]Raw, error) { return nil, errors.New("registry unreadable") }
func (failingSys) Getenv(string) string { return "" }

func TestSetModeChangesOnlyTheStartModeAndRemembersTheOriginal(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetMode("Steam Client Service", Disabled, false); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.Mode("Steam Client Service"); got != Disabled {
		t.Fatalf("mode = %q", got)
	}
	if !reflect.DeepEqual(r.fake.Writes, []string{"Steam Client Service=disabled"}) {
		t.Fatalf("writes = %v: exactly one start-mode write and nothing else", r.fake.Writes)
	}
	if v, ok, _ := r.store.Get("steam client service"); !ok || v != "manual" {
		t.Fatalf("original = %q, %v", v, ok)
	}
	items, _ := r.m.List()
	for _, it := range items {
		if it.ID == "Steam Client Service" && (!it.Changed || it.Original != Manual || it.StartMode != Disabled) {
			t.Fatalf("item = %+v", it)
		}
	}
	// A second change keeps the first original.
	if err := r.m.SetMode("Steam Client Service", Auto, false); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := r.store.Get("Steam Client Service"); v != "manual" {
		t.Fatalf("original after second change = %q, want manual", v)
	}
}

func TestSetModeKeepsTheDelayedFlagInTheOriginal(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetMode("OneSync", Manual, false); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := r.store.Get("OneSync"); v != "delayed" {
		t.Fatalf("original = %q, want delayed", v)
	}
	if err := r.m.Restore("OneSync", false); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.Mode("OneSync"); got != Delayed {
		t.Fatalf("restored mode = %q, want delayed", got)
	}
	if _, ok, _ := r.store.Get("OneSync"); ok {
		t.Fatal("restoring forgets the original")
	}
	items, _ := r.m.List()
	for _, it := range items {
		if it.ID == "OneSync" && it.Changed {
			t.Fatal("a restored service is not 'changed'")
		}
	}
}

func TestChangingBackByHandForgetsTheOriginal(t *testing.T) {
	r := newRig(t)
	_ = r.m.SetMode("Steam Client Service", Disabled, false)
	if err := r.m.SetMode("Steam Client Service", Manual, false); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.store.Get("Steam Client Service"); ok {
		t.Fatal("back at the original: nothing to restore")
	}
}

func TestSetModeSameModeWritesNothing(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetMode("Steam Client Service", Manual, false); err != nil {
		t.Fatal(err)
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("no write expected")
	}
	if _, ok, _ := r.store.Get("Steam Client Service"); ok {
		t.Fatal("nothing should be remembered for a no-op")
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		name string
		id   string
		mode Mode
		pre  func(*rig)
		want error
	}{
		{"unknown service", "Nope", Manual, nil, ErrNotFound},
		{"protected by name", "WinDefend", Disabled, nil, ErrProtected},
		{"windows service", "Dhcp", Disabled, nil, ErrProtected},
		{"driver", "npcap", Disabled, nil, ErrProtected},
		{"protected, guessed in other case", "windefend", Disabled, nil, ErrProtected},
		{"bad mode", "Steam Client Service", Mode("stopped"), nil, ErrBadMode},
		{"empty mode", "Steam Client Service", Mode(""), nil, ErrBadMode},
		{"not administrator", "Steam Client Service", Disabled, func(r *rig) { r.admin = false }, ErrNeedsAdmin},
		{"security product asks first", "AVP21", Disabled, nil, ErrNeedsConfirm},
		{"security product asks even for manual", "AVP21", Manual, nil, ErrNeedsConfirm},
		{"dependents ask first when lowering", "Parent", Manual, nil, ErrHasDependents},
		{"dependents ask first when disabling", "Parent", Disabled, nil, ErrHasDependents},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			if c.pre != nil {
				c.pre(r)
			}
			err := r.m.SetMode(c.id, c.mode, false)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if r.fake.WriteCount() != 0 {
				t.Fatal("a refused change must write nothing")
			}
			if all, _ := r.store.All(); len(all) != 0 {
				t.Fatalf("a refused change must remember nothing: %v", all)
			}
		})
	}
}

func TestProtectedIsRefusedEvenWhenConfirmed(t *testing.T) {
	r := newRig(t)
	for _, id := range []string{"WinDefend", "Dhcp", "npcap"} {
		if err := r.m.SetMode(id, Disabled, true); !errors.Is(err, ErrProtected) {
			t.Fatalf("%s: err = %v", id, err)
		}
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestConfirmedChangesGoThrough(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetMode("AVP21", Manual, true); err != nil {
		t.Fatal(err)
	}
	if err := r.m.SetMode("Parent", Disabled, true); err != nil {
		t.Fatal(err)
	}
	if r.fake.Mode("AVP21") != Manual || r.fake.Mode("Parent") != Disabled {
		t.Fatal("confirmed changes should apply")
	}
}

func TestDependentsOnlyAskWhenLowering(t *testing.T) {
	r := newRig(t)
	// Parent is Automatic: Automatic -> Delayed is not lowering.
	if err := r.m.SetMode("Parent", Delayed, false); err != nil {
		t.Fatalf("auto -> delayed should not ask: %v", err)
	}
	// Child is Manual with no dependents: raising never asks.
	if err := r.m.SetMode("Child", Auto, false); err != nil {
		t.Fatal(err)
	}
	// Raising a service that has dependents does not ask either.
	r.fake.Services[3].Start = 4 // Parent disabled by someone else
	if err := r.m.SetMode("Parent", Auto, false); err != nil {
		t.Fatalf("raising should not ask: %v", err)
	}
}

func TestAChangeThatDoesNotStickIsReported(t *testing.T) {
	r := newRig(t)
	r.fake.IgnoreSets = true
	if err := r.m.SetMode("Steam Client Service", Disabled, false); !errors.Is(err, ErrNotApplied) {
		t.Fatalf("err = %v, want not-applied", err)
	}
}

func TestAFailedWriteForgetsTheOriginalItAdded(t *testing.T) {
	r := newRig(t)
	r.fake.SetErr = errors.New("svc:failed: boom")
	if err := r.m.SetMode("Steam Client Service", Disabled, false); err == nil {
		t.Fatal("want the write error")
	}
	if _, ok, _ := r.store.Get("Steam Client Service"); ok {
		t.Fatal("nothing changed, so nothing should be remembered")
	}
}

func TestNoChangeWithoutAStoreToRememberIt(t *testing.T) {
	r := newRig(t)
	r.m.Store = nil
	if err := r.m.SetMode("Steam Client Service", Disabled, false); !errors.Is(err, ErrStore) {
		t.Fatalf("err = %v", err)
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("without a way to restore, nothing is changed")
	}
}

func TestAnUnwritableStoreRefusesTheChange(t *testing.T) {
	r := newRig(t)
	// A directory where the file should be makes every write fail.
	r.m.Store = &origstate.Store{Path: t.TempDir()}
	if err := r.m.SetMode("Steam Client Service", Disabled, false); !errors.Is(err, ErrStore) {
		t.Fatalf("err = %v", err)
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestRestoreRefusals(t *testing.T) {
	r := newRig(t)
	if err := r.m.Restore("Steam Client Service", false); !errors.Is(err, ErrNoOriginal) {
		t.Fatalf("nothing remembered: %v", err)
	}
	_ = r.m.SetMode("Steam Client Service", Disabled, false)
	r.admin = false
	if err := r.m.Restore("Steam Client Service", false); !errors.Is(err, ErrNeedsAdmin) {
		t.Fatalf("restore needs admin too: %v", err)
	}
	r.admin = true
	// A remembered id for something that became hidden is refused.
	if _, err := r.store.Remember("Dhcp", "auto"); err != nil {
		t.Fatal(err)
	}
	if err := r.m.Restore("Dhcp", true); !errors.Is(err, ErrProtected) {
		t.Fatalf("restore of a protected service: %v", err)
	}
}

func TestRestoreOfASecurityProductAsksFirst(t *testing.T) {
	r := newRig(t)
	_ = r.m.SetMode("AVP21", Manual, true)
	if err := r.m.Restore("AVP21", false); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("err = %v", err)
	}
	if err := r.m.Restore("AVP21", true); err != nil {
		t.Fatal(err)
	}
	if r.fake.Mode("AVP21") != Auto {
		t.Fatal("should be back to automatic")
	}
}

func TestADamagedOriginalsFileDoesNotBlockChangesOrLists(t *testing.T) {
	r := newRig(t)
	if _, err := r.store.Remember("x", "auto"); err != nil {
		t.Fatal(err)
	}
	// Corrupt it on disk.
	if err := os.WriteFile(r.store.Path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.List(); err != nil {
		t.Fatal(err)
	}
	if err := r.m.SetMode("Steam Client Service", Disabled, false); err != nil {
		t.Fatal(err)
	}
}

func TestRememberedModeThatIsInvalidIsIgnored(t *testing.T) {
	r := newRig(t)
	_, _ = r.store.Remember("Steam Client Service", "garbage")
	items, _ := r.m.List()
	for _, it := range items {
		if it.ID == "Steam Client Service" && it.Changed {
			t.Fatal("an invalid remembered value must not show Restore")
		}
	}
	if err := r.m.Restore("Steam Client Service", true); !errors.Is(err, ErrNoOriginal) {
		t.Fatalf("err = %v", err)
	}
}
