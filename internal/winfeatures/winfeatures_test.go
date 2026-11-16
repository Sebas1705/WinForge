package winfeatures

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

// fake is an in-memory machine: nothing here touches Windows.
type fake struct {
	states     map[string]int
	statesErr  error
	pending    map[string]string
	pendingErr error
	pendingAsk int
	wslOut     map[string]string // joined args -> text wsl.exe prints
	wslCode    map[string]int
	wslMissing bool
	defVer     int
	calls      []string
}

func (f *fake) OptionalFeatures() (map[string]int, error) { return f.states, f.statesErr }
func (f *fake) PendingFeatures() (map[string]string, error) {
	f.pendingAsk++
	return f.pending, f.pendingErr
}
func (f *fake) WSL(args ...string) ([]byte, int, error) {
	if f.wslMissing {
		return nil, 0, ErrWSLMissing
	}
	k := strings.Join(args, " ")
	f.calls = append(f.calls, k)
	return utf16le(f.wslOut[k], false), f.wslCode[k], nil
}
func (f *fake) WSLDefaultVersion() (int, bool) { return f.defVer, f.defVer != 0 }

// utf16le encodes text the way wsl.exe prints it: CRLF lines, UTF-16LE, optionally with a BOM.
func utf16le(s string, bom bool) []byte {
	s = strings.ReplaceAll(s, "\n", "\r\n")
	var b []byte
	if bom {
		b = append(b, 0xFF, 0xFE)
	}
	for _, c := range utf16.Encode([]rune(s)) {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

func byID(t *testing.T, fs []Feature) map[string]Feature {
	t.Helper()
	m := map[string]Feature{}
	for _, f := range fs {
		m[f.ID] = f
	}
	if len(m) != 5 || len(fs) != 5 {
		t.Fatalf("want the five features once each: %+v", fs)
	}
	return m
}

func proStates() map[string]int {
	return map[string]int{
		"Microsoft-Windows-Subsystem-Linux": 1,
		"VirtualMachinePlatform":            1,
		"Microsoft-Hyper-V-All":             2,
		"Containers-DisposableClientVM":     2,
		"NetFx3":                            3,
	}
}

func TestListMapsEveryInstallState(t *testing.T) {
	got := byID(t, List(&fake{states: proStates()}, true))
	want := map[string]string{"wsl": On, "vmp": On, "hyperv": Off, "sandbox": Off, "netfx3": Unavailable}
	for id, st := range want {
		if got[id].State != st {
			t.Errorf("%s: state %q, want %q", id, got[id].State, st)
		}
		if got[id].Enable != "feature-"+id+"-on" || got[id].Disable != "feature-"+id+"-off" {
			t.Errorf("%s: recipe ids %q %q", id, got[id].Enable, got[id].Disable)
		}
		if got[id].NeedsAdmin || got[id].RestartPending {
			t.Errorf("%s: elevated and nothing pending: %+v", id, got[id])
		}
	}
	if fs := List(&fake{states: map[string]int{"NetFx3": 99}}, true); byID(t, fs)["netfx3"].State != Unknown {
		t.Error("an InstallState Windows does not define is unknown")
	}
}

func TestListHomeEditionHasNoHyperVOrSandbox(t *testing.T) {
	home := map[string]int{"Microsoft-Windows-Subsystem-Linux": 2, "VirtualMachinePlatform": 2, "NetFx3": 2, "Printing-PrintToPDFServices-Features": 1}
	got := byID(t, List(&fake{states: home}, true))
	if got["hyperv"].State != Unavailable || got["sandbox"].State != Unavailable {
		t.Errorf("features missing from Windows' list do not exist on this edition: %+v", got)
	}
	if got["wsl"].State != Off || got["vmp"].State != Off || got["netfx3"].State != Off {
		t.Errorf("the others are readable: %+v", got)
	}
}

func TestListNotElevatedNeedsAdminAndNeverAsksForPending(t *testing.T) {
	f := &fake{states: proStates(), pending: map[string]string{"NetFx3": PendingEnable}}
	got := byID(t, List(f, false))
	for id, ft := range got {
		if !ft.NeedsAdmin {
			t.Errorf("%s: changing it needs administrator rights", id)
		}
		if ft.RestartPending {
			t.Errorf("%s: Windows only reports pending restarts to administrators", id)
		}
	}
	if f.pendingAsk != 0 {
		t.Error("PendingFeatures must not be called when not elevated")
	}
	if got["wsl"].State != On {
		t.Errorf("states are readable without administrator rights: %+v", got["wsl"])
	}
}

func TestListUnreadableIsUnknownNotAnError(t *testing.T) {
	for name, f := range map[string]*fake{
		"error": {statesErr: errors.New("access denied")},
		"empty": {states: map[string]int{}},
		"nil":   {},
	} {
		for _, ft := range List(f, false) {
			if ft.State != Unknown {
				t.Errorf("%s: %s is %q, want unknown", name, ft.ID, ft.State)
			}
		}
	}
}

func TestListRestartPending(t *testing.T) {
	f := &fake{
		states:  proStates(),
		pending: map[string]string{"Microsoft-Hyper-V-All": PendingEnable, "VirtualMachinePlatform": PendingDisable},
	}
	got := byID(t, List(f, true))
	if h := got["hyperv"]; h.State != On || !h.RestartPending {
		t.Errorf("a feature that waits to turn on is on with a restart pending: %+v", h)
	}
	if v := got["vmp"]; v.State != Off || !v.RestartPending {
		t.Errorf("a feature that waits to turn off is off with a restart pending: %+v", v)
	}
	if w := got["wsl"]; w.RestartPending {
		t.Errorf("nothing pending for wsl: %+v", w)
	}
	// If Windows cannot say what is pending, the states still show.
	f2 := &fake{states: proStates(), pendingErr: errors.New("needs admin")}
	if byID(t, List(f2, true))["wsl"].State != On {
		t.Error("pending failure must not hide the state")
	}
}

func TestListJSONShape(t *testing.T) {
	b, err := json.Marshal(List(&fake{states: proStates()}, false)[0])
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"id":"wsl","state":"on","restartPending":false,"enable":"feature-wsl-on","disable":"feature-wsl-off","needsAdmin":true}`
	if string(b) != want {
		t.Fatalf("%s\nwant %s", b, want)
	}
}

const enTable = ` NAME                   STATE           VERSION
* Ubuntu-24.04          Running         2
  docker-desktop        Stopped         2
  Debian                Stopped         1
`

func TestParseDistrosEnglishWithAndWithoutBOM(t *testing.T) {
	for _, bom := range []bool{false, true} {
		ds := ParseDistros(utf16le(enTable, bom))
		want := []Distro{
			{"Ubuntu-24.04", "Running", 2, true},
			{"docker-desktop", "Stopped", 2, false},
			{"Debian", "Stopped", 1, false},
		}
		if len(ds) != len(want) {
			t.Fatalf("bom=%v: %+v", bom, ds)
		}
		for i := range want {
			if ds[i] != want[i] {
				t.Errorf("bom=%v row %d: %+v, want %+v", bom, i, ds[i], want[i])
			}
		}
	}
}

func TestParseDistrosLocalizedHeaderAndStates(t *testing.T) {
	es := table("NOMBRE", "ESTADO", "VERSIÓN", [][3]string{{"* Ubuntu", "En ejecución", "2"}, {"  Alpine", "Detenido", "2"}})
	ds := ParseDistros(utf16le(es, false))
	if len(ds) != 2 {
		t.Fatalf("%+v", ds)
	}
	if ds[0] != (Distro{"Ubuntu", "En ejecución", 2, true}) || ds[1] != (Distro{"Alpine", "Detenido", 2, false}) {
		t.Fatalf("columns are found by position, whatever the language: %+v", ds)
	}
	zh := table("名称", "状态", "版本", [][3]string{{"* Ubuntu", "正在运行", "2"}})
	if ds := ParseDistros(utf16le(zh, true)); len(ds) != 1 || ds[0].Name != "Ubuntu" || ds[0].Version != 2 || !ds[0].IsDefault {
		t.Fatalf("%+v", ds)
	}
}

func TestParseDistrosUTF8AndOddLayouts(t *testing.T) {
	if ds := ParseDistros([]byte(enTable)); len(ds) != 3 {
		t.Errorf("WSL_UTF8=1 output must parse too: %+v", ds)
	}
	// Columns that do not line up with the header fall back to reading from the end of the line.
	ragged := " NAME   STATE   VERSION\n* A-very-long-distribution-name Stopped 2\n"
	ds := ParseDistros([]byte(ragged))
	if len(ds) != 1 || ds[0].Name != "A-very-long-distribution-name" || ds[0].State != "Stopped" || ds[0].Version != 2 {
		t.Errorf("%+v", ds)
	}
	// Blank lines and trailing spaces are harmless.
	if ds := ParseDistros([]byte("\n  NAME   STATE   VERSION  \n\n* Ubuntu   Stopped   2   \n\n")); len(ds) != 1 {
		t.Errorf("%+v", ds)
	}
}

func TestParseDistrosNoTable(t *testing.T) {
	for name, out := range map[string][]byte{
		"empty":     nil,
		"none":      utf16le("Windows Subsystem for Linux has no installed distributions.\nUse 'wsl.exe --list --online' to list available distributions\nand 'wsl.exe --install <Distro>' to install.\n", false),
		"not there": utf16le("The Windows Subsystem for Linux is not installed. You can install by running 'wsl.exe --install'.\nFor more information please visit https://aka.ms/wslinstall\n", false),
		"spanish":   utf16le("El Subsistema de Windows para Linux no tiene distribuciones instaladas.\n", true),
	} {
		ds := ParseDistros(out)
		if len(ds) != 0 {
			t.Errorf("%s: %+v", name, ds)
		}
		if ds == nil {
			t.Errorf("%s: the list must not be nil, so JSON shows []", name)
		}
	}
}

func TestWSLRunningStateComesFromTheRunningList(t *testing.T) {
	es := table("NOMBRE", "ESTADO", "VERSIÓN", [][3]string{{"* Ubuntu", "En ejecución", "2"}, {"  Alpine", "Detenido", "2"}})
	f := &fake{
		states: proStates(),
		defVer: 2,
		wslOut: map[string]string{"-l -v": es, "-l --running -q": "Ubuntu\n"},
	}
	info := WSL(f)
	if !info.Installed || info.DefaultVersion != 2 || len(info.Distros) != 2 {
		t.Fatalf("%+v", info)
	}
	if info.Distros[0].State != "Running" || info.Distros[1].State != "Stopped" {
		t.Fatalf("states are normalized so the page need not know every language: %+v", info.Distros)
	}
	for _, c := range f.calls {
		if c != "-l -v" && c != "-l --running -q" {
			t.Errorf("only listings may run, got wsl %s", c)
		}
	}
}

func TestWSLKeepsRawStateWhenTheRunningListFails(t *testing.T) {
	f := &fake{
		states:  proStates(),
		wslOut:  map[string]string{"-l -v": enTable},
		wslCode: map[string]int{"-l --running -q": 1},
	}
	info := WSL(f)
	if info.Distros[0].State != "Running" || info.Distros[1].State != "Stopped" {
		t.Fatalf("%+v", info.Distros)
	}
}

func TestWSLMissingOrEmpty(t *testing.T) {
	missing := WSL(&fake{states: proStates(), wslMissing: true})
	if missing.Installed || missing.Distros == nil || len(missing.Distros) != 0 || missing.DefaultVersion != 2 {
		t.Errorf("wsl.exe missing: %+v", missing)
	}
	none := "Windows Subsystem for Linux has no installed distributions.\n"
	empty := WSL(&fake{states: proStates(), wslOut: map[string]string{"-l -v": none}, wslCode: map[string]int{"-l -v": 1}})
	if !empty.Installed || len(empty.Distros) != 0 {
		t.Errorf("WSL on but no distribution: installed, empty list: %+v", empty)
	}
	off := WSL(&fake{states: map[string]int{"Microsoft-Windows-Subsystem-Linux": 2}, wslOut: map[string]string{"-l -v": "WSL is not installed.\n"}})
	if off.Installed || len(off.Distros) != 0 {
		t.Errorf("feature off and no distributions: not installed: %+v", off)
	}
	unreadable := WSL(&fake{statesErr: errors.New("x"), wslOut: map[string]string{"-l -v": enTable}})
	if !unreadable.Installed || len(unreadable.Distros) != 3 {
		t.Errorf("distributions prove WSL is there even when features cannot be read: %+v", unreadable)
	}
}

func TestWSLDefaultVersion(t *testing.T) {
	if v := WSL(&fake{defVer: 1, wslMissing: true}).DefaultVersion; v != 1 {
		t.Errorf("default version 1: %d", v)
	}
	if v := WSL(&fake{defVer: 7, wslMissing: true}).DefaultVersion; v != 2 {
		t.Errorf("nonsense falls back to 2: %d", v)
	}
	if v := WSL(&fake{wslMissing: true}).DefaultVersion; v != 2 {
		t.Errorf("never chosen means 2: %d", v)
	}
}

func TestWSLInfoJSONShape(t *testing.T) {
	f := &fake{states: proStates(), defVer: 2, wslOut: map[string]string{"-l -v": enTable}}
	b, err := json.Marshal(WSL(f))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), `{"installed":true,"defaultVersion":2,"distros":[{"name":"Ubuntu-24.04","state":"Running","version":2,"isDefault":true},`) {
		t.Fatalf("%s", b)
	}
	if b, _ := json.Marshal(WSL(&fake{wslMissing: true})); string(b) != `{"installed":false,"defaultVersion":2,"distros":[]}` {
		t.Fatalf("%s", b)
	}
}

func TestDecodeOutputGuessesTheEncoding(t *testing.T) {
	if got := DecodeOutput(utf16le("héllo\nx", false)); got != "héllo\nx" {
		t.Errorf("%q", got)
	}
	if got := DecodeOutput([]byte("plain\r\nutf8 é")); got != "plain\nutf8 é" {
		t.Errorf("%q", got)
	}
	if got := DecodeOutput([]byte{0xEF, 0xBB, 0xBF, 'o', 'k'}); got != "ok" {
		t.Errorf("%q", got)
	}
	if DecodeOutput(nil) != "" {
		t.Error("nil is empty")
	}
}

// table lays out a `wsl -l -v` table the way wsl.exe does: the columns start at the same rune
// offset in the header and in every row.
func table(name, state, version string, rows [][3]string) string {
	pad := func(s string, n int) string {
		for len([]rune(s)) < n {
			s += " "
		}
		return s
	}
	out := pad("  "+name, 24) + pad(state, 16) + version + "\n"
	for _, r := range rows {
		out += pad(r[0], 24) + pad(r[1], 16) + r[2] + "\n"
	}
	return out
}

func TestWindowsName(t *testing.T) {
	if WindowsName("hyperv") != "Microsoft-Hyper-V-All" || WindowsName("sandbox") != "Containers-DisposableClientVM" || WindowsName("nope") != "" {
		t.Error("feature names")
	}
}
