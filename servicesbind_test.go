package main

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/services"
)

func svcRaw(name, display string, start uint32, image string, deps ...string) services.Raw {
	return services.Raw{Name: name, DisplayName: display, Type: 0x10, Start: start, ImagePath: image, DependsOn: deps}
}

// svcApp is an App on a fake machine: nothing here touches a real service or the real %AppData%.
func svcApp(t *testing.T) (*App, *services.Fake) {
	f := services.NewFake()
	f.Services = []services.Raw{
		svcRaw("Steam Client Service", "Steam Client Service", 3, `"C:\Program Files (x86)\Steam\bin\steamservice.exe"`),
		svcRaw("AVP21", "Kaspersky Service", 2, `"C:\Program Files\Kaspersky Lab\avp.exe"`),
		svcRaw("Parent", "Vendor Parent", 2, `"C:\Program Files\Vendor\parent.exe"`),
		svcRaw("Child", "Vendor Child", 3, `"C:\Program Files\Vendor\child.exe"`, "Parent"),
		svcRaw("WinDefend", "Microsoft Defender Antivirus Service", 2, `C:\ProgramData\Microsoft\Windows Defender\MsMpEng.exe`),
		svcRaw("Dhcp", "DHCP Client", 2, `C:\Windows\system32\svchost.exe -k x`),
	}
	a := &App{}
	dir := t.TempDir()
	a.svc.servicesSys = f
	a.svc.stateDir = func() (string, error) { return dir, nil }
	a.elevated = func() bool { return true }
	return a, f
}

func svcIDs(items []services.Item) string {
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return strings.Join(ids, ",")
}

func TestServicesListHidesProtectedAndDrivers(t *testing.T) {
	a, _ := svcApp(t)
	items, err := a.ServicesList()
	if err != nil {
		t.Fatal(err)
	}
	if got := svcIDs(items); got != "AVP21,Steam Client Service,Child,Parent" {
		t.Fatalf("ids = %s", got)
	}
}

func TestServicesSetModeRefusesWhatWasNotListed(t *testing.T) {
	a, f := svcApp(t)
	// Never listed at all.
	if _, err := a.ServicesSetMode("Steam Client Service", "disabled", false); !errors.Is(err, errServiceNotListed) {
		t.Fatalf("before any list: %v", err)
	}
	if _, err := a.ServicesList(); err != nil {
		t.Fatal(err)
	}
	// Hidden and protected ones are not in the list, so a guessed id is refused.
	for _, id := range []string{"WinDefend", "windefend", "Dhcp", "Nope", ""} {
		if _, err := a.ServicesSetMode(id, "disabled", true); !errors.Is(err, errServiceNotListed) {
			t.Fatalf("%q: %v", id, err)
		}
		if _, err := a.ServicesRestore(id, true); !errors.Is(err, errServiceNotListed) {
			t.Fatalf("restore %q: %v", id, err)
		}
	}
	if f.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestServicesSetModeRechecksProtectionAtWriteTime(t *testing.T) {
	a, f := svcApp(t)
	if _, err := a.ServicesList(); err != nil {
		t.Fatal(err)
	}
	// Between the list and the change the service turns out to be a Windows one (its program moved
	// under %WINDIR%): the write-time check refuses it even though its id was listed.
	f.Services[0].ImagePath = `C:\Windows\system32\steam.exe`
	if _, err := a.ServicesSetMode("Steam Client Service", "disabled", true); !errors.Is(err, services.ErrProtected) {
		t.Fatalf("err = %v", err)
	}
	if f.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
	// A refusal as protected drops the id: it cannot be retried without a fresh list.
	if _, err := a.ServicesSetMode("Steam Client Service", "disabled", true); !errors.Is(err, errServiceNotListed) {
		t.Fatalf("retry: %v", err)
	}
}

func TestServicesSetModeChangesAndReturnsTheRefreshedList(t *testing.T) {
	a, f := svcApp(t)
	_, _ = a.ServicesList()
	items, err := a.ServicesSetMode("Steam Client Service", "delayed", false)
	if err != nil {
		t.Fatal(err)
	}
	var steam services.Item
	for _, it := range items {
		if it.ID == "Steam Client Service" {
			steam = it
		}
	}
	if steam.StartMode != services.Delayed || !steam.Changed || steam.Original != services.Manual {
		t.Fatalf("steam = %+v", steam)
	}
	if f.WriteCount() != 1 {
		t.Fatalf("writes = %v", f.Writes)
	}
	// Restore.
	items, err = a.ServicesRestore("steam client service", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == "Steam Client Service" && (it.StartMode != services.Manual || it.Changed) {
			t.Fatalf("restored = %+v", it)
		}
	}
}

func TestServicesSetModeNeedsAdminConfirmationAndAValidMode(t *testing.T) {
	a, f := svcApp(t)
	_, _ = a.ServicesList()
	a.elevated = func() bool { return false }
	if _, err := a.ServicesSetMode("Steam Client Service", "disabled", false); !errors.Is(err, services.ErrNeedsAdmin) {
		t.Fatalf("admin: %v", err)
	}
	a.elevated = func() bool { return true }
	if _, err := a.ServicesSetMode("AVP21", "disabled", false); !errors.Is(err, services.ErrNeedsConfirm) {
		t.Fatalf("security: %v", err)
	}
	if _, err := a.ServicesSetMode("Parent", "manual", false); !errors.Is(err, services.ErrHasDependents) {
		t.Fatalf("dependents: %v", err)
	}
	for _, bad := range []string{"", "stopped", "AUTO", "start", "boot"} {
		if _, err := a.ServicesSetMode("Steam Client Service", bad, true); !errors.Is(err, services.ErrBadMode) {
			t.Fatalf("mode %q: %v", bad, err)
		}
	}
	if f.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
	if _, err := a.ServicesSetMode("Parent", "manual", true); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
}

func TestServicesSetModeOnAGoneServiceDropsItFromTheAllowList(t *testing.T) {
	a, f := svcApp(t)
	_, _ = a.ServicesList()
	f.Services = f.Services[1:] // Steam disappears
	if _, err := a.ServicesSetMode("Steam Client Service", "disabled", false); !errors.Is(err, services.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if a.servicesIsListed("Steam Client Service") {
		t.Fatal("a gone service must leave the allow-list")
	}
}

func TestServicesChangesAreOneAtATime(t *testing.T) {
	a, f := svcApp(t)
	_, _ = a.ServicesList()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mode := []string{"disabled", "auto", "manual", "delayed"}[i%4]
			_, _ = a.ServicesSetMode("Steam Client Service", mode, true)
		}(i)
	}
	wg.Wait()
	if !f.Mode("Steam Client Service").Valid() {
		t.Fatal("the service ends in a valid mode")
	}
	// The original is the very first mode, whichever change ran first.
	if o, ok, _ := a.origStore("services-original.json").Get("Steam Client Service"); ok && o != "manual" {
		t.Fatalf("original = %q, want manual", o)
	}
}

func TestServicesListErrorIsReturned(t *testing.T) {
	a, _ := svcApp(t)
	a.svc.servicesSys = svcBroken{}
	if _, err := a.ServicesList(); err == nil {
		t.Fatal("want the read error")
	}
}

type svcBroken struct{ *services.Fake }

func (svcBroken) List() ([]services.Raw, error) { return nil, errors.New("scm unavailable") }
func (svcBroken) Getenv(string) string          { return "" }
