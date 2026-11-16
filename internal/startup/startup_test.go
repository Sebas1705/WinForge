package startup

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fake is an in-memory machine. It records every write so tests can check that only
// StartupApproved values are ever touched.
type fake struct {
	entries  map[Origin][]Raw
	approved map[Origin]map[string][]byte // by lower-case name
	files    map[string]bool
	env      map[string]string
	info     map[string]FileInfo
	readErr  map[Origin]error
	writes   []string // "<origin>/<name>"
	inspects int
}

func newFake() *fake {
	return &fake{
		entries: map[Origin][]Raw{}, approved: map[Origin]map[string][]byte{}, files: map[string]bool{},
		env: map[string]string{}, info: map[string]FileInfo{}, readErr: map[Origin]error{},
	}
}

func (f *fake) Entries(o Origin) ([]Raw, error) {
	if err := f.readErr[o]; err != nil {
		return nil, err
	}
	return append([]Raw(nil), f.entries[o]...), nil
}

func (f *fake) Approved(o Origin) (map[string][]byte, error) {
	out := map[string][]byte{}
	for k, v := range f.approved[o] {
		out[k] = append([]byte(nil), v...)
	}
	return out, nil
}

func (f *fake) SetApproved(o Origin, name string, data []byte) error {
	if f.approved[o] == nil {
		f.approved[o] = map[string][]byte{}
	}
	f.approved[o][strings.ToLower(name)] = append([]byte(nil), data...)
	f.writes = append(f.writes, string(o)+"/"+name)
	return nil
}

func (f *fake) Exists(p string) bool   { return f.files[strings.ToLower(p)] }
func (f *fake) Getenv(n string) string { return f.env[strings.ToUpper(n)] }
func (f *fake) Inspect(paths []string) map[string]FileInfo {
	f.inspects++
	out := map[string]FileInfo{}
	for _, p := range paths {
		if i, ok := f.info[p]; ok {
			out[p] = i
		}
	}
	return out
}

func (f *fake) addRun(o Origin, name, cmd string) {
	f.entries[o] = append(f.entries[o], Raw{name, cmd})
}
func (f *fake) addFile(p string) { f.files[strings.ToLower(p)] = true }
func (f *fake) setApproved(o Origin, name string, b []byte) {
	if f.approved[o] == nil {
		f.approved[o] = map[string][]byte{}
	}
	f.approved[o][strings.ToLower(name)] = b
}

var t0 = time.Date(2026, 11, 15, 10, 30, 0, 0, time.UTC)

func mgr(f *fake, admin bool) *Manager {
	return &Manager{Sys: f, Elevated: func() bool { return admin }, Now: func() time.Time { return t0 }}
}

func TestExpandPercent(t *testing.T) {
	env := map[string]string{"PROGRAMFILES": `C:\Program Files`, "APPDATA": `C:\Users\me\AppData\Roaming`}
	lookup := func(n string) string { return env[strings.ToUpper(n)] }
	cases := []struct{ in, want string }{
		{``, ``},
		{`plain`, `plain`},
		{`%ProgramFiles%\A\a.exe`, `C:\Program Files\A\a.exe`},
		{`%programfiles%\A`, `C:\Program Files\A`},
		{`%APPDATA%\x\%PROGRAMFILES%`, `C:\Users\me\AppData\Roaming\x\C:\Program Files`},
		{`%UNKNOWN%\a.exe`, `%UNKNOWN%\a.exe`},
		{`100% sure %APPDATA%`, `100% sure C:\Users\me\AppData\Roaming`},
		{`%`, `%`},
		{`%%`, `%%`},
		{`a%b`, `a%b`},
		{`%APPDATA`, `%APPDATA`},
	}
	for _, c := range cases {
		if got := ExpandPercent(c.in, lookup); got != c.want {
			t.Errorf("ExpandPercent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	exists := func(p string) bool {
		switch strings.ToLower(p) {
		case `c:\program files\my app\my app.exe`, `c:\program files\x\x.exe`, `c:\tools\t.exe`, `c:\program files\y\y.exe`:
			return true
		}
		return false
	}
	env := map[string]string{"PROGRAMFILES": `C:\Program Files`}
	lookup := func(n string) string { return env[strings.ToUpper(n)] }
	cases := []struct {
		name, in string
		want     Parsed
	}{
		{"empty", ``, Parsed{}},
		{"spaces only", `   `, Parsed{}},
		{"quoted no args", `"C:\Program Files\X\x.exe"`, Parsed{Path: `C:\Program Files\X\x.exe`, Checkable: true}},
		{"quoted with args", `"C:\Program Files\X\x.exe" --minimized /tray`, Parsed{Path: `C:\Program Files\X\x.exe`, Args: `--minimized /tray`, Checkable: true}},
		{"quoted missing close", `"C:\Tools\t.exe -x`, Parsed{Path: `C:\Tools\t.exe -x`, Checkable: true}},
		{"unquoted simple", `C:\Tools\t.exe`, Parsed{Path: `C:\Tools\t.exe`, Checkable: true}},
		{"unquoted with args", `C:\Tools\t.exe -silent`, Parsed{Path: `C:\Tools\t.exe`, Args: `-silent`, Checkable: true}},
		{"unquoted path with spaces", `C:\Program Files\My App\My App.exe`, Parsed{Path: `C:\Program Files\My App\My App.exe`, Checkable: true}},
		{"unquoted spaces and args", `C:\Program Files\My App\My App.exe /background`, Parsed{Path: `C:\Program Files\My App\My App.exe`, Args: `/background`, Checkable: true}},
		{"unquoted missing file keeps extension split", `C:\Program Files\Gone\gone.exe /x`, Parsed{Path: `C:\Program Files\Gone\gone.exe`, Args: `/x`, Checkable: true}},
		{"unquoted missing no extension", `C:\Program Files\Gone`, Parsed{Path: `C:\Program`, Args: `Files\Gone`, Checkable: true}},
		{"exe added", `C:\Tools\t`, Parsed{Path: `C:\Tools\t.exe`, Checkable: true}},
		{"env var", `%ProgramFiles%\X\x.exe -a`, Parsed{Path: `C:\Program Files\X\x.exe`, Args: `-a`, Checkable: true}},
		{"env var quoted", `"%ProgramFiles%\Y\y.exe"`, Parsed{Path: `C:\Program Files\Y\y.exe`, Checkable: true}},
		{"unset var stays", `%NOPE%\z.exe`, Parsed{Path: `%NOPE%\z.exe`}},
		{"bare name", `notepad.exe`, Parsed{Path: `notepad.exe`}},
		{"bare name args", `notepad.exe C:\a.txt`, Parsed{Path: `notepad.exe`, Args: `C:\a.txt`}},
		{"rundll32", `rundll32.exe C:\Windows\System32\foo.dll,Start`, Parsed{Args: `rundll32.exe C:\Windows\System32\foo.dll,Start`, Launcher: true}},
		{"rundll32 full path quoted", `"C:\Windows\System32\RUNDLL32.EXE" shell32.dll,Control_RunDLL`, Parsed{Args: `C:\Windows\System32\RUNDLL32.EXE shell32.dll,Control_RunDLL`, Launcher: true}},
		{"cmd /c", `cmd /c start "" "C:\a b\c.exe"`, Parsed{Args: `cmd /c start "" "C:\a b\c.exe"`, Launcher: true}},
		{"cmd.exe /c", `C:\Windows\system32\cmd.exe /c "echo hi"`, Parsed{Args: `C:\Windows\system32\cmd.exe /c "echo hi"`, Launcher: true}},
		{"powershell", `powershell -NoProfile -File C:\s.ps1`, Parsed{Args: `powershell -NoProfile -File C:\s.ps1`, Launcher: true}},
		{"tabs between words", "C:\\Tools\\t.exe\t-x", Parsed{Path: `C:\Tools\t.exe`, Args: "\t-x", Checkable: true}},
		{"unc", `\\server\share\a.exe /q`, Parsed{Path: `\\server\share\a.exe`, Args: `/q`, Checkable: true}},
		{"forward slashes", `C:/Tools/t.exe -a`, Parsed{Path: `C:/Tools/t.exe`, Args: `-a`, Checkable: true}},
	}
	for _, c := range cases {
		got := ParseCommand(c.in, lookup, exists)
		// Args are trimmed by the parser.
		c.want.Args = strings.TrimSpace(c.want.Args)
		if got != c.want {
			t.Errorf("%s: ParseCommand(%q) = %+v, want %+v", c.name, c.in, got, c.want)
		}
	}
	if got := ParseCommand(`C:\a b\c.exe -x`, nil, nil); got.Path != `C:\a b\c.exe` {
		t.Errorf("nil exists should still split on the extension, got %+v", got)
	}
}

func TestApprovedEncoding(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want bool
	}{
		{"absent", nil, true},
		{"empty", []byte{}, true},
		{"enabled 02", []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"enabled 06", []byte{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"disabled 03", []byte{3, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, false},
		{"disabled 07", []byte{7, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, false},
		{"one byte is damaged: enabled", []byte{3}, true},
		{"three bytes damaged: enabled", []byte{3, 0, 0}, true},
		{"four bytes odd", []byte{3, 0, 0, 0}, false},
		{"long blob", append([]byte{3, 0, 0, 0}, make([]byte, 40)...), false},
	}
	for _, c := range cases {
		if got := DecodeApproved(c.b); got != c.want {
			t.Errorf("%s: DecodeApproved = %v, want %v", c.name, got, c.want)
		}
	}

	on := EncodeApproved(true, t0)
	if !bytes.Equal(on, []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("enabled blob = % x", on)
	}
	off := EncodeApproved(false, t0)
	if len(off) != 12 || off[0] != 3 || off[1] != 0 || off[2] != 0 || off[3] != 0 {
		t.Fatalf("disabled blob = % x", off)
	}
	if DecodeApproved(off) {
		t.Error("a disabled blob must decode as disabled")
	}
	at, ok := DisabledAt(off)
	if !ok || !at.Equal(t0) {
		t.Errorf("DisabledAt = %v %v, want %v", at, ok, t0)
	}
	// 2026-11-15T10:30:00Z as a FILETIME, computed independently:
	// (unix 1794738600 s * 10^7) + 116444736000000000.
	ft := uint64(1794738600)*10_000_000 + 116444736000000000
	want := make([]byte, 8)
	for i := range want {
		want[i] = byte(ft >> (8 * i))
	}
	if t0.Unix() != 1794738600 {
		t.Fatalf("test constant is off: %d", t0.Unix())
	}
	if !bytes.Equal(off[4:], want) {
		t.Errorf("FILETIME bytes = % x, want % x", off[4:], want)
	}
	if _, ok := DisabledAt(on); ok {
		t.Error("an enabled blob has no disable time")
	}
	if _, ok := DisabledAt([]byte{3, 0, 0, 0}); ok {
		t.Error("a short blob has no disable time")
	}
	if _, ok := DisabledAt([]byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); ok {
		t.Error("a zero FILETIME is no time")
	}
}

func TestCaution(t *testing.T) {
	cases := []struct {
		name, pub, cmd string
		want           bool
	}{
		{"SecurityHealth", "", "", true},
		{"SecurityHealth", "Microsoft Corporation", `%windir%\system32\SecurityHealthSystray.exe`, true},
		{"x", "", `C:\Windows\System32\SecurityHealthSystray.exe`, true},
		{"Windows Defender", "", "", true},
		{"OneDrive", "Microsoft Corporation", "", true},
		{"Teams", "microsoft corporation", "", true},
		{"Discord", "Discord Inc.", `C:\Users\me\Discord\Update.exe`, false},
		{"Steam", "Valve Corporation", "", false},
		{"MsMpEng helper", "", "", true},
		{"", "", "", false},
		{"Notmicrosoft", "Acme Microsoftware", "", false},
	}
	for _, c := range cases {
		if got := Caution(c.name, c.pub, c.cmd); got != c.want {
			t.Errorf("Caution(%q,%q,%q) = %v, want %v", c.name, c.pub, c.cmd, got, c.want)
		}
	}
}

func TestIDs(t *testing.T) {
	for _, name := range []string{"Simple", "with|bar", "|", "ünïcode ✓", `a"b'c`, "a b", "dots.lnk"} {
		for _, o := range Origins {
			id := MakeID(o, name)
			o2, n2, ok := ParseID(id)
			if !ok || o2 != o || n2 != name {
				t.Errorf("round trip of %q gave %v %q %v", id, o2, n2, ok)
			}
		}
	}
	for _, bad := range []string{"", "nobar", "|x", "bogus|x", "hkcu-run|", "HKCU-RUN|x"} {
		if _, _, ok := ParseID(bad); ok {
			t.Errorf("ParseID(%q) should fail", bad)
		}
	}
}

func TestOriginMapping(t *testing.T) {
	cases := []struct {
		o       Origin
		machine bool
		folder  bool
		key     string
	}{
		{UserRun, false, false, "Run"},
		{MachineRun, true, false, "Run"},
		{MachineRun32, true, false, "Run32"},
		{UserFolder, false, true, "StartupFolder"},
		{CommonFolder, true, true, "StartupFolder"},
	}
	for _, c := range cases {
		if c.o.Machine() != c.machine || c.o.Folder() != c.folder || c.o.ApprovedKey() != c.key {
			t.Errorf("%s: machine=%v folder=%v key=%s", c.o, c.o.Machine(), c.o.Folder(), c.o.ApprovedKey())
		}
	}
}

func byID(items []Item, id string) (Item, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

func TestListEmpty(t *testing.T) {
	items, err := mgr(newFake(), false).List()
	if err != nil || len(items) != 0 {
		t.Fatalf("empty machine: %v %v", items, err)
	}
	if items == nil {
		t.Error("the list must be empty, not nil, so the interface gets []")
	}
}

func TestListFields(t *testing.T) {
	f := newFake()
	f.env["PROGRAMFILES"] = `C:\Program Files`
	f.addFile(`C:\Program Files\Acme\acme.exe`)
	f.info[`C:\Program Files\Acme\acme.exe`] = FileInfo{Publisher: " Acme Inc. "}
	f.addRun(UserRun, "Acme", `"%ProgramFiles%\Acme\acme.exe" --tray`)
	f.addRun(UserRun, "Gone", `"C:\Program Files\Gone\gone.exe"`)
	f.addRun(UserRun, "Dll", `rundll32.exe foo.dll,Run`)
	f.addRun(UserRun, "Bare", `notepad.exe`)
	f.addRun(MachineRun, "SecurityHealth", `%windir%\system32\SecurityHealthSystray.exe`)
	f.addRun(MachineRun32, "Old", `C:\Old\old.exe`)
	f.addFile(`C:\Old\old.exe`)
	f.setApproved(UserRun, "Acme", EncodeApproved(false, t0))
	f.setApproved(MachineRun32, "old", []byte{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	items, err := mgr(f, false).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("got %d items: %+v", len(items), items)
	}
	acme, _ := byID(items, "hkcu-run|Acme")
	if acme.Enabled || acme.Publisher != "Acme Inc." || acme.Missing || acme.Target != `C:\Program Files\Acme\acme.exe` || acme.NeedsAdmin || acme.Caution {
		t.Errorf("acme = %+v", acme)
	}
	if acme.Command != `"%ProgramFiles%\Acme\acme.exe" --tray` {
		t.Errorf("the command must stay as written: %q", acme.Command)
	}
	gone, _ := byID(items, "hkcu-run|Gone")
	if !gone.Missing || !gone.Enabled {
		t.Errorf("gone = %+v", gone)
	}
	dll, _ := byID(items, "hkcu-run|Dll")
	if dll.Missing || dll.Target != "" || dll.Publisher != "" {
		t.Errorf("a launcher is neither checked nor given a publisher: %+v", dll)
	}
	bare, _ := byID(items, "hkcu-run|Bare")
	if bare.Missing {
		t.Errorf("a bare program name is found through PATH, not reported missing: %+v", bare)
	}
	sec, _ := byID(items, "hklm-run|SecurityHealth")
	if !sec.NeedsAdmin || !sec.Caution || !sec.Enabled {
		t.Errorf("security = %+v", sec)
	}
	old, _ := byID(items, "hklm-run32|Old")
	if !old.NeedsAdmin || !old.Enabled {
		t.Errorf("old = %+v", old)
	}
	if f.inspects != 1 {
		t.Errorf("publishers must be read in one batch, got %d calls", f.inspects)
	}
	if len(f.writes) != 0 {
		t.Errorf("listing must not write: %v", f.writes)
	}
}

func TestListFolders(t *testing.T) {
	f := newFake()
	f.addRun(UserFolder, "Tool.lnk", `C:\Users\me\Startup\Tool.lnk`)
	f.addFile(`C:\Users\me\Startup\Tool.lnk`)
	f.info[`C:\Users\me\Startup\Tool.lnk`] = FileInfo{Publisher: "Tool Co", Target: `C:\Tool\tool.exe`}
	f.addRun(UserFolder, "Dead.lnk", `C:\Users\me\Startup\Dead.lnk`)
	f.addFile(`C:\Users\me\Startup\Dead.lnk`)
	f.info[`C:\Users\me\Startup\Dead.lnk`] = FileInfo{Target: `C:\Dead\dead.exe`}
	f.addRun(UserFolder, "run me.bat", `C:\Users\me\Startup\run me.bat`)
	f.addFile(`C:\Users\me\Startup\run me.bat`)
	f.addRun(CommonFolder, "Shared.URL", `C:\ProgramData\Startup\Shared.URL`)
	f.addFile(`C:\ProgramData\Startup\Shared.URL`)
	f.addFile(`C:\Tool\tool.exe`)
	f.setApproved(UserFolder, "tool.lnk", EncodeApproved(false, t0))

	items, err := mgr(f, false).List()
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := byID(items, "user-folder|Tool.lnk")
	if !ok || tool.Name != "Tool" || tool.Enabled || tool.Publisher != "Tool Co" || tool.Target != `C:\Tool\tool.exe` || tool.Missing {
		t.Errorf("tool = %+v", tool)
	}
	dead, _ := byID(items, "user-folder|Dead.lnk")
	if !dead.Missing {
		t.Errorf("a shortcut to a missing program is a broken entry: %+v", dead)
	}
	bat, _ := byID(items, "user-folder|run me.bat")
	if bat.Name != "run me.bat" || bat.Missing || bat.Target != `C:\Users\me\Startup\run me.bat` {
		t.Errorf("bat = %+v", bat)
	}
	shared, _ := byID(items, "common-folder|Shared.URL")
	if shared.Name != "Shared" || !shared.NeedsAdmin {
		t.Errorf("shared = %+v", shared)
	}
}

func TestListDuplicateNamesAcrossSources(t *testing.T) {
	f := newFake()
	for _, o := range []Origin{UserRun, MachineRun, MachineRun32} {
		f.addRun(o, "Updater", `C:\u.exe`)
	}
	f.addRun(UserRun, "UPDATER", `C:\other.exe`) // same value name in the same key: Windows keeps one
	f.setApproved(MachineRun, "Updater", EncodeApproved(false, t0))
	items, err := mgr(f, false).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %+v", items)
	}
	ids := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{"hkcu-run|Updater", "hklm-run|Updater", "hklm-run32|Updater"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("order = %v, want %v", ids, want)
		}
	}
	// Each source has its own flag: only the HKLM Run one is off.
	if !items[0].Enabled || items[1].Enabled || !items[2].Enabled {
		t.Errorf("flags are per source: %+v", items)
	}
}

func TestListOddNames(t *testing.T) {
	f := newFake()
	names := []string{`Ünï côdé ✓`, `with "quotes"`, `a|b`, `  spaced  `, `日本語`, `very ` + strings.Repeat("long ", 80)}
	for _, n := range names {
		f.addRun(UserRun, n, `C:\x.exe`)
	}
	f.addRun(UserRun, "", `C:\empty-name.exe`)
	items, err := mgr(f, false).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(names) {
		t.Fatalf("want %d items (the unnamed value is not an item), got %d", len(names), len(items))
	}
	for _, n := range names {
		if _, ok := byID(items, MakeID(UserRun, n)); !ok {
			t.Errorf("missing %q", n)
		}
	}
}

func TestListNoApprovedValueIsEnabled(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "A", `C:\a.exe`)
	items, _ := mgr(f, false).List()
	if len(items) != 1 || !items[0].Enabled {
		t.Fatalf("%+v", items)
	}
}

func TestListMalformedApproved(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "Short", `C:\a.exe`)
	f.addRun(UserRun, "Empty", `C:\b.exe`)
	f.addRun(UserRun, "Four", `C:\c.exe`)
	f.setApproved(UserRun, "Short", []byte{3})
	f.setApproved(UserRun, "Empty", []byte{})
	f.setApproved(UserRun, "Four", []byte{3, 0, 0, 0})
	items, err := mgr(f, false).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   string
		want bool
	}{{"hkcu-run|Short", true}, {"hkcu-run|Empty", true}, {"hkcu-run|Four", false}} {
		it, _ := byID(items, c.id)
		if it.Enabled != c.want {
			t.Errorf("%s enabled = %v, want %v", c.id, it.Enabled, c.want)
		}
	}
}

func TestListOrder(t *testing.T) {
	f := newFake()
	f.addRun(MachineRun, "beta", `C:\b.exe`)
	f.addRun(UserRun, "Alpha", `C:\a.exe`)
	f.addRun(UserRun, "alpine", `C:\a.exe`)
	f.addRun(UserFolder, "Beta.lnk", `C:\s\Beta.lnk`)
	items, _ := mgr(f, false).List()
	var got []string
	for _, it := range items {
		got = append(got, it.ID)
	}
	want := []string{"hkcu-run|Alpha", "hkcu-run|alpine", "hklm-run|beta", "user-folder|Beta.lnk"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestListPartialFailure(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "A", `C:\a.exe`)
	f.readErr[MachineRun] = errors.New("access denied")
	items, err := mgr(f, false).List()
	if err != nil || len(items) != 1 {
		t.Fatalf("one unreadable origin must not hide the rest: %v %v", items, err)
	}
	for _, o := range Origins {
		f.readErr[o] = errors.New("boom " + string(o))
	}
	if _, err := mgr(f, false).List(); err == nil {
		t.Error("when nothing can be read the error is reported")
	}
}

func TestListLong(t *testing.T) {
	f := newFake()
	for i := 0; i < 200; i++ {
		o := Origins[i%len(Origins)]
		name := fmt.Sprintf("Item %03d", i)
		if o.Folder() {
			name += ".lnk"
			f.addRun(o, name, `C:\s\`+name)
			f.addFile(`C:\s\` + name)
		} else {
			f.addRun(o, name, fmt.Sprintf(`"C:\Apps\a%d.exe" -x`, i))
		}
		if i%3 == 0 {
			f.setApproved(o, name, EncodeApproved(false, t0))
		}
	}
	items, err := mgr(f, true).List()
	if err != nil || len(items) != 200 {
		t.Fatalf("%d items, err %v", len(items), err)
	}
	if f.inspects != 1 {
		t.Errorf("one batch expected, got %d", f.inspects)
	}
	off := 0
	for _, it := range items {
		if !it.Enabled {
			off++
		}
	}
	if off != 67 {
		t.Errorf("switched off = %d, want 67", off)
	}
}

func TestToggleWritesOnlyTheFlag(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "App", `C:\a.exe`)
	f.addRun(MachineRun32, "Old", `C:\o.exe`)
	f.addRun(UserFolder, "S.lnk", `C:\s\S.lnk`)
	f.addRun(CommonFolder, "C.lnk", `C:\c\C.lnk`)
	m := mgr(f, true)

	for _, c := range []struct {
		id    string
		write string
	}{
		{"hkcu-run|App", "hkcu-run/App"},
		{"hklm-run32|Old", "hklm-run32/Old"},
		{"user-folder|S.lnk", "user-folder/S.lnk"},
		{"common-folder|C.lnk", "common-folder/C.lnk"},
	} {
		f.writes = nil
		if err := m.Toggle(c.id, false); err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		if len(f.writes) != 1 || f.writes[0] != c.write {
			t.Errorf("%s: writes = %v", c.id, f.writes)
		}
		o, name, _ := ParseID(c.id)
		got := f.approved[o][strings.ToLower(name)]
		if !bytes.Equal(got, EncodeApproved(false, t0)) {
			t.Errorf("%s: blob = % x", c.id, got)
		}
	}
	// The Run values and files themselves are untouched.
	if len(f.entries[UserRun]) != 1 || f.entries[UserRun][0].Command != `C:\a.exe` {
		t.Errorf("entries changed: %+v", f.entries)
	}
	items, _ := m.List()
	for _, it := range items {
		if it.Enabled {
			t.Errorf("%s should be off", it.ID)
		}
	}
	for _, it := range items {
		if err := m.Toggle(it.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	items, _ = m.List()
	for _, it := range items {
		if !it.Enabled {
			t.Errorf("%s should be on again", it.ID)
		}
	}
	if got := f.approved[UserRun]["app"]; !bytes.Equal(got, []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("enabled blob = % x", got)
	}
}

func TestToggleIdempotent(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "App", `C:\a.exe`)
	m := mgr(f, false)

	// Enabling an item with no value writes nothing.
	if err := m.Toggle("hkcu-run|App", true); err != nil || len(f.writes) != 0 {
		t.Fatalf("enable absent: %v %v", err, f.writes)
	}
	if err := m.Toggle("hkcu-run|App", false); err != nil || len(f.writes) != 1 {
		t.Fatalf("disable: %v %v", err, f.writes)
	}
	// Twice: the second call keeps the first FILETIME.
	m.Now = func() time.Time { return t0.Add(time.Hour) }
	if err := m.Toggle("hkcu-run|App", false); err != nil || len(f.writes) != 1 {
		t.Fatalf("disable twice: %v %v", err, f.writes)
	}
	if at, _ := DisabledAt(f.approved[UserRun]["app"]); !at.Equal(t0) {
		t.Errorf("time changed to %v", at)
	}
	if err := m.Toggle("hkcu-run|App", true); err != nil || len(f.writes) != 2 {
		t.Fatalf("enable: %v %v", err, f.writes)
	}
	if err := m.Toggle("hkcu-run|App", true); err != nil || len(f.writes) != 2 {
		t.Fatalf("enable twice: %v %v", err, f.writes)
	}
	// An enabled flag written by Windows (06) is not rewritten.
	f.setApproved(UserRun, "App", []byte{6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if err := m.Toggle("hkcu-run|App", true); err != nil || len(f.writes) != 2 {
		t.Fatalf("enable 06: %v %v", err, f.writes)
	}
	// A disabled flag written by Windows (07) is not rewritten either.
	f.setApproved(UserRun, "App", []byte{7, 0, 0, 0, 9, 9, 9, 9, 9, 9, 9, 9})
	if err := m.Toggle("hkcu-run|App", false); err != nil || len(f.writes) != 2 {
		t.Fatalf("disable 07: %v %v", err, f.writes)
	}
}

func TestToggleMalformedBlob(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "App", `C:\a.exe`)
	f.setApproved(UserRun, "App", []byte{3})
	m := mgr(f, false)
	if err := m.Toggle("hkcu-run|App", false); err != nil {
		t.Fatal(err)
	}
	if got := f.approved[UserRun]["app"]; len(got) != 12 || DecodeApproved(got) {
		t.Errorf("a damaged value is replaced by a whole one: % x", got)
	}
}

func TestToggleMachineNeedsAdmin(t *testing.T) {
	f := newFake()
	f.addRun(MachineRun, "M", `C:\m.exe`)
	f.addRun(MachineRun32, "M32", `C:\m.exe`)
	f.addRun(CommonFolder, "F.lnk", `C:\f.lnk`)
	f.addRun(UserRun, "U", `C:\u.exe`)
	m := mgr(f, false)
	for _, id := range []string{"hklm-run|M", "hklm-run32|M32", "common-folder|F.lnk"} {
		for _, on := range []bool{true, false} {
			err := m.Toggle(id, on)
			if !errors.Is(err, ErrNeedsAdmin) || !strings.Contains(err.Error(), "needs administrator") {
				t.Errorf("%s: %v", id, err)
			}
		}
	}
	if len(f.writes) != 0 {
		t.Errorf("nothing may be written without admin: %v", f.writes)
	}
	if err := m.Toggle("hkcu-run|U", false); err != nil {
		t.Errorf("user items need no admin: %v", err)
	}
	m.Elevated = nil
	if err := m.Toggle("hklm-run|M", false); !errors.Is(err, ErrNeedsAdmin) {
		t.Errorf("nil Elevated means not elevated: %v", err)
	}
	m.Elevated = func() bool { return true }
	if err := m.Toggle("hklm-run|M", false); err != nil {
		t.Errorf("elevated: %v", err)
	}
}

func TestToggleUnknown(t *testing.T) {
	f := newFake()
	f.addRun(UserRun, "A", `C:\a.exe`)
	m := mgr(f, true)
	for _, id := range []string{"", "A", "hkcu-run|B", "bogus|A", "hklm-run|A", "hkcu-run|", "|A"} {
		if err := m.Toggle(id, false); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: %v", id, err)
		}
	}
	if len(f.writes) != 0 {
		t.Errorf("writes: %v", f.writes)
	}
	// The value name matches the way the registry does: ignoring case, writing the real case.
	if err := m.Toggle("hkcu-run|a", false); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 1 || f.writes[0] != "hkcu-run/A" {
		t.Errorf("writes = %v", f.writes)
	}
	// The item disappears between listing and switching.
	f.entries[UserRun] = nil
	if err := m.Toggle("hkcu-run|A", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("gone: %v", err)
	}
}

func TestToggleReadError(t *testing.T) {
	f := newFake()
	f.readErr[UserRun] = errors.New("denied")
	if err := mgr(f, true).Toggle("hkcu-run|A", false); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("a read error is not 'not found': %v", err)
	}
}
