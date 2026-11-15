package main

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/startup"
)

// fakeStartup is an in-memory machine for the Startup page: no registry, no disk.
type fakeStartup struct {
	mu       sync.Mutex
	entries  map[startup.Origin][]startup.Raw
	approved map[startup.Origin]map[string][]byte
	files    map[string]bool
	writes   []string
}

func newFakeStartup() *fakeStartup {
	return &fakeStartup{
		entries:  map[startup.Origin][]startup.Raw{},
		approved: map[startup.Origin]map[string][]byte{},
		files:    map[string]bool{},
	}
}

func (f *fakeStartup) Entries(o startup.Origin) ([]startup.Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]startup.Raw(nil), f.entries[o]...), nil
}

func (f *fakeStartup) Approved(o startup.Origin) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]byte{}
	for k, v := range f.approved[o] {
		out[k] = append([]byte(nil), v...)
	}
	return out, nil
}

func (f *fakeStartup) SetApproved(o startup.Origin, name string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.approved[o] == nil {
		f.approved[o] = map[string][]byte{}
	}
	f.approved[o][strings.ToLower(name)] = append([]byte(nil), data...)
	f.writes = append(f.writes, string(o)+"/"+name)
	return nil
}

func (f *fakeStartup) Exists(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[strings.ToLower(p)]
}
func (f *fakeStartup) Getenv(string) string                         { return "" }
func (f *fakeStartup) Inspect([]string) map[string]startup.FileInfo { return nil }

func (f *fakeStartup) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

func startupApp(t *testing.T, admin bool) (*App, *fakeStartup) {
	t.Helper()
	a := testApp(t)
	f := newFakeStartup()
	f.entries[startup.UserRun] = []startup.Raw{{Name: "Chat", Command: `"C:\Apps\chat.exe" --tray`}, {Name: "Gone", Command: `C:\Gone\gone.exe`}}
	f.entries[startup.MachineRun] = []startup.Raw{{Name: "Audio", Command: `C:\Audio\audio.exe`}}
	f.entries[startup.UserFolder] = []startup.Raw{{Name: "Notes.lnk", Command: `C:\Startup\Notes.lnk`}}
	f.files[`c:\apps\chat.exe`] = true
	f.files[`c:\audio\audio.exe`] = true
	f.files[`c:\startup\notes.lnk`] = true
	a.startupSys = f
	a.elevated = func() bool { return admin }
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	return a, f
}

func findItem(items []startup.Item, id string) (startup.Item, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return startup.Item{}, false
}

func TestStartupListShowsEverythingOnce(t *testing.T) {
	a, f := startupApp(t, false)
	items, err := a.StartupList()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %+v", items)
	}
	gone, _ := findItem(items, "hkcu-run|Gone")
	if !gone.Missing {
		t.Errorf("a missing file is flagged: %+v", gone)
	}
	audio, _ := findItem(items, "hklm-run|Audio")
	if !audio.NeedsAdmin {
		t.Errorf("a machine item needs admin: %+v", audio)
	}
	if f.writeCount() != 0 {
		t.Error("listing writes nothing")
	}
}

func TestStartupListEmpty(t *testing.T) {
	a := testApp(t)
	a.startupSys = newFakeStartup()
	items, err := a.StartupList()
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("%v %v", items, err)
	}
}

func TestStartupSetOnlyListedIDs(t *testing.T) {
	a, f := startupApp(t, true)
	// Before any list, nothing can be switched.
	if _, err := a.StartupSet("hkcu-run|Chat", false); err == nil || !strings.Contains(err.Error(), "not in the last list") {
		t.Fatalf("unlisted: %v", err)
	}
	if _, err := a.StartupList(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "Chat", "hkcu-run|Other", "hklm-run32|Chat", "../../x", "hkcu-run|chat"} {
		if _, err := a.StartupSet(id, false); err == nil || !strings.Contains(err.Error(), "not in the last list") {
			t.Errorf("%q must be refused, got %v", id, err)
		}
	}
	if f.writeCount() != 0 {
		t.Errorf("refused ids write nothing: %v", f.writes)
	}
	// A valid id is accepted and returns the refreshed list.
	items, err := a.StartupSet("hkcu-run|Chat", false)
	if err != nil {
		t.Fatal(err)
	}
	chat, _ := findItem(items, "hkcu-run|Chat")
	if chat.Enabled || len(items) != 4 {
		t.Errorf("chat = %+v of %d", chat, len(items))
	}
}

func TestStartupSetRefusesWhatTheNewListNoLongerHas(t *testing.T) {
	a, f := startupApp(t, true)
	a.StartupList()
	// Something else adds a value after the list: it was never shown, so it is refused.
	f.entries[startup.UserRun] = append(f.entries[startup.UserRun], startup.Raw{Name: "Sneaky", Command: `C:\s.exe`})
	if _, err := a.StartupSet("hkcu-run|Sneaky", false); err == nil || !strings.Contains(err.Error(), "not in the last list") {
		t.Errorf("%v", err)
	}
	// An item that was listed but is gone when switched is reported and forgotten.
	f.entries[startup.UserRun] = nil
	if _, err := a.StartupSet("hkcu-run|Chat", false); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("%v", err)
	}
	if _, err := a.StartupSet("hkcu-run|Chat", false); err == nil || !strings.Contains(err.Error(), "not in the last list") {
		t.Errorf("forgotten after it vanished: %v", err)
	}
	if f.writeCount() != 0 {
		t.Error("nothing written")
	}
}

func TestStartupSetMachineNeedsAdmin(t *testing.T) {
	a, f := startupApp(t, false)
	a.StartupList()
	for _, on := range []bool{false, true} {
		_, err := a.StartupSet("hklm-run|Audio", on)
		if err == nil || !strings.Contains(err.Error(), "needs administrator") {
			t.Errorf("on=%v: %v", on, err)
		}
	}
	if f.writeCount() != 0 {
		t.Errorf("no partial write: %v", f.writes)
	}
	a.elevated = func() bool { return true }
	items, err := a.StartupSet("hklm-run|Audio", false)
	if err != nil {
		t.Fatal(err)
	}
	audio, _ := findItem(items, "hklm-run|Audio")
	if audio.Enabled || len(f.writes) != 1 || f.writes[0] != "hklm-run/Audio" {
		t.Errorf("audio = %+v writes = %v", audio, f.writes)
	}
}

func TestStartupSetTwiceAndBack(t *testing.T) {
	a, f := startupApp(t, false)
	a.StartupList()
	for i := 0; i < 2; i++ {
		if _, err := a.StartupSet("user-folder|Notes.lnk", false); err != nil {
			t.Fatal(err)
		}
	}
	if f.writeCount() != 1 {
		t.Errorf("switching off twice writes once, got %v", f.writes)
	}
	want := startup.EncodeApproved(false, time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC))
	if got := f.approved[startup.UserFolder]["notes.lnk"]; !bytes.Equal(got, want) {
		t.Errorf("blob = % x", got)
	}
	items, err := a.StartupSet("user-folder|Notes.lnk", true)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := findItem(items, "user-folder|Notes.lnk")
	if !n.Enabled || f.writeCount() != 2 {
		t.Errorf("%+v %v", n, f.writes)
	}
	if _, err := a.StartupSet("user-folder|Notes.lnk", true); err != nil || f.writeCount() != 2 {
		t.Errorf("switching on twice: %v %v", err, f.writes)
	}
	// The entries themselves were never touched.
	if len(f.entries[startup.UserFolder]) != 1 || len(f.entries[startup.UserRun]) != 2 {
		t.Error("entries changed")
	}
}

func TestStartupSetConcurrent(t *testing.T) {
	a, f := startupApp(t, true)
	a.StartupList()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(on bool) {
			defer wg.Done()
			if _, err := a.StartupSet("hkcu-run|Chat", on); err != nil {
				t.Error(err)
			}
		}(i%2 == 0)
	}
	wg.Wait()
	if f.writeCount() < 1 {
		t.Error("something was written")
	}
}

func TestStartupReveal(t *testing.T) {
	a, _ := startupApp(t, false)
	var shown []string
	a.reveal = func(p string) error { shown = append(shown, p); return nil }
	if err := a.StartupReveal("hkcu-run|Chat"); err == nil || !strings.Contains(err.Error(), "not in the last list") {
		t.Fatalf("unlisted: %v", err)
	}
	a.StartupList()
	if err := a.StartupReveal("hkcu-run|Chat"); err != nil || len(shown) != 1 || shown[0] != `C:\Apps\chat.exe` {
		t.Fatalf("%v %v", err, shown)
	}
	if err := a.StartupReveal("user-folder|Notes.lnk"); err != nil || shown[1] != `C:\Startup\Notes.lnk` {
		t.Fatalf("%v %v", err, shown)
	}
	// A broken entry has nothing to show.
	if err := a.StartupReveal("hkcu-run|Gone"); err == nil || !strings.Contains(err.Error(), "not there") {
		t.Errorf("%v", err)
	}
	// Ids the list did not return are refused, and nothing is revealed for them.
	for _, id := range []string{`C:\Windows\System32\cmd.exe`, "hkcu-run|Nope", ""} {
		if err := a.StartupReveal(id); err == nil {
			t.Errorf("%q must be refused", id)
		}
	}
	if len(shown) != 2 {
		t.Errorf("shown = %v", shown)
	}
	// A launcher has no file to show.
	f := a.startupSys.(*fakeStartup)
	f.entries[startup.UserRun] = append(f.entries[startup.UserRun], startup.Raw{Name: "Dll", Command: `rundll32.exe x.dll,Go`})
	a.StartupList()
	if err := a.StartupReveal("hkcu-run|Dll"); err == nil {
		t.Error("a launcher has nothing to reveal")
	}
	a.reveal = func(string) error { return errors.New("explorer failed") }
	if err := a.StartupReveal("hkcu-run|Chat"); err == nil || !strings.Contains(err.Error(), "explorer failed") {
		t.Errorf("%v", err)
	}
}
