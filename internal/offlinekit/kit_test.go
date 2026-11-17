package offlinekit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

var root = filepath.Join(os.TempDir(), "kit-test")

// manifestYAML is a merged winget manifest the way `winget download` writes it.
func manifestYAML(id, version, typ, file string, installer []byte, extra string) []byte {
	return []byte(fmt.Sprintf(`PackageIdentifier: %s
PackageVersion: %s
PackageLocale: en-US
Publisher: Vendor
Installers:
- Architecture: x64
  InstallerType: %s
  InstallerSha256: %s
  InstallerUrl: https://example.com/%s
%s
ManifestType: merged
ManifestVersion: 1.28.0
`, id, version, typ, strings.ToUpper(sum(installer)), file, extra))
}

// pkg is what a download of one package leaves in its folder.
func pkg(id, version, typ, ext string, extra string) (map[string][]byte, []byte) {
	installer := []byte("installer of " + id + " " + version)
	base := strings.ReplaceAll(id, ".", "_") + "_" + version
	return map[string][]byte{
		base + "_X64." + ext: installer,
		base + "_X64.yaml":   manifestYAML(id, version, typ, base+"."+ext, installer, extra),
	}, installer
}

func testCatalog() *catalog.Catalog {
	mk := func(id, name, winget string, admin bool) *catalog.App {
		return &catalog.App{ID: id, Name: name, Winget: winget, Admin: admin}
	}
	return &catalog.Catalog{Apps: map[string]*catalog.App{
		"alpha":  mk("alpha", "Alpha", "Vendor.Alpha", false),
		"beta":   mk("beta", "Beta", "Vendor.Beta", true),
		"gamma":  mk("gamma", "Gamma", "Vendor.Gamma", false),
		"store":  mk("store", "Store app", "9NBLGGH4NNS1", false),
		"zipper": mk("zipper", "Zipper", "Vendor.Zipper", false),
		"fail":   mk("fail", "Failing", "Vendor.Fail", false),
		"noswi":  mk("noswi", "NoSwitches", "Vendor.NoSwitches", false),
	}}
}

func known(cat *catalog.Catalog) Known {
	return func(id, winget string) bool {
		a := cat.Apps[id]
		return a != nil && strings.EqualFold(a.Winget, winget)
	}
}

func fixedNow() time.Time { return time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC) }

// newKit makes a kit with alpha (exe, inno), beta (msi) through the real Create and a fake downloader.
func newKit(t *testing.T) (*MemFS, *Result) {
	t.Helper()
	fs := NewMemFS()
	alpha, _ := pkg("Vendor.Alpha", "1.2.3", "inno", "exe", "")
	beta, _ := pkg("Vendor.Beta", "4.5", "msi", "msi", "")
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{
		"Vendor.Alpha": {Files: alpha}, "Vendor.Beta": {Files: beta},
	}}
	res, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1.0.0", Now: fixedNow}, []string{"alpha", "beta"}, root, "Profile")
	if err != nil {
		t.Fatal(err)
	}
	return fs, res
}

func verify(t *testing.T, fs FS) *Report {
	t.Helper()
	rep, err := Verify(context.Background(), fs, root, known(testCatalog()), nil)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func problemKinds(a AppReport) []string {
	var out []string
	for _, p := range a.Problems {
		out = append(out, p.Kind+":"+p.File)
	}
	return out
}

func find(rep *Report, id string) AppReport {
	for _, a := range rep.Apps {
		if a.Entry.ID == id {
			return a
		}
	}
	return AppReport{}
}

func TestParseManifestLikeWingetWrites(t *testing.T) {
	// Rufus.Rufus 4.16, as `winget download` wrote it on this PC (trimmed): the version is a bare number.
	y := `PackageIdentifier: Rufus.Rufus
PackageVersion: 4.16
Installers:
- Architecture: x64
  InstallerType: portable
  InstallerSha256: 8351a56c134aa6dd26a930d702cc3fd57f11ef90b17e55a8de960d5a73d46cfc
  InstallerUrl: https://github.com/pbatard/rufus/releases/download/v4.16/rufus-4.16.exe
  Commands:
  - rufus
ManifestType: merged
`
	m, err := ParseManifest([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "Rufus.Rufus" || m.Version != "4.16" || len(m.Installers) != 1 {
		t.Fatalf("parsed %+v", m)
	}
	in, ok := m.Select("8351A56C134AA6DD26A930D702CC3FD57F11EF90B17E55A8DE960D5A73D46CFC")
	if !ok || in.Type != "portable" || in.Arch != "x64" {
		t.Fatalf("select %+v %v", in, ok)
	}
	if _, ok := m.Select(strings.Repeat("0", 64)); ok {
		t.Fatal("a file whose hash the manifest does not pin was accepted")
	}
	if Classify("portable", "rufus.exe") != ClassNo {
		t.Fatal("portable must not be run")
	}
}

func TestParseManifestSwitchesAndDefaults(t *testing.T) {
	y := `PackageIdentifier: A.B
PackageVersion: "1.0"
InstallerType: exe
Scope: machine
InstallerSwitches:
  Silent: /quiet
  Custom: /X=1
InstallerSuccessCodes:
- 5
Installers:
- Architecture: x64
  InstallerSha256: AA
  InstallerSwitches:
    SilentWithProgress: /passive
- Architecture: x86
  InstallerType: nullsoft
  InstallerSha256: BB
  ElevationRequirement: elevationRequired
`
	m, err := ParseManifest([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	a, b := m.Installers[0], m.Installers[1]
	if a.Type != "exe" || a.Scope != "machine" || a.Silent != "/passive" || a.Custom != "/X=1" || !reflect.DeepEqual(a.Codes, []int{5}) || a.Elevation {
		t.Fatalf("a = %+v", a)
	}
	if b.Type != "nullsoft" || b.Silent != "/quiet" || !b.Elevation {
		t.Fatalf("b = %+v", b)
	}
	if _, err := ParseManifest([]byte("PackageIdentifier: A.B\n")); err == nil {
		t.Fatal("a manifest without installers was accepted")
	}
	if _, err := ParseManifest([]byte(":::not yaml")); err == nil {
		t.Fatal("garbage was accepted")
	}
}

func TestBuildCommandTable(t *testing.T) {
	const spaced = `C:\Temp\kit copy\My App 1.0 "x".exe`
	cases := []struct {
		name string
		in   Installer
		path string
		want Command
		err  bool
	}{
		{"msi", Installer{File: "a.msi", Type: "msi"}, `C:\k\a.msi`, Command{"msiexec", []string{"/i", `C:\k\a.msi`, "/qn", "/norestart"}}, false},
		{"wix is msi", Installer{File: "a.msi", Type: "wix"}, `C:\k\a.msi`, Command{"msiexec", []string{"/i", `C:\k\a.msi`, "/qn", "/norestart"}}, false},
		{"msi custom properties", Installer{File: "a.msi", Type: "msi", Custom: "ALLUSERS=1 INSTALLDIR=\"C:\\Program Files\\A\""}, `C:\k\a.msi`,
			Command{"msiexec", []string{"/i", `C:\k\a.msi`, "/qn", "/norestart", "ALLUSERS=1", `INSTALLDIR=C:\Program Files\A`}}, false},
		{"msi path with spaces stays one argument", Installer{File: "a.msi", Type: "msi"}, `C:\Temp\kit copy\a b.msi`, Command{"msiexec", []string{"/i", `C:\Temp\kit copy\a b.msi`, "/qn", "/norestart"}}, false},
		{"inno default", Installer{File: "a.exe", Type: "inno"}, `C:\k\a.exe`, Command{`C:\k\a.exe`, []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}}, false},
		{"nullsoft default", Installer{File: "a.exe", Type: "nullsoft"}, spaced, Command{spaced, []string{"/S"}}, false},
		{"burn default", Installer{File: "a.exe", Type: "burn"}, `C:\k\a.exe`, Command{`C:\k\a.exe`, []string{"/quiet", "/norestart"}}, false},
		{"exe uses the manifest switch", Installer{File: "a.exe", Type: "exe", Silent: "/silent /nodesktop", Custom: "/LANG=es"}, `C:\k\a.exe`, Command{`C:\k\a.exe`, []string{"/silent", "/nodesktop", "/LANG=es"}}, false},
		{"manifest switch beats the default", Installer{File: "a.exe", Type: "nullsoft", Silent: "/S /D=1"}, `C:\k\a.exe`, Command{`C:\k\a.exe`, []string{"/S", "/D=1"}}, false},
		{"exe without switches", Installer{File: "a.exe", Type: "exe"}, `C:\k\a.exe`, Command{}, true},
		{"placeholder switch", Installer{File: "a.exe", Type: "exe", Silent: "/D=<INSTALLPATH>"}, `C:\k\a.exe`, Command{}, true},
		{"unbalanced quote", Installer{File: "a.exe", Type: "exe", Silent: `/S "/D=x`}, `C:\k\a.exe`, Command{}, true},
		{"msix", Installer{File: "a.msix", Type: "msix"}, `C:\k\a.msix`, Command{}, true},
		{"zip", Installer{File: "a.zip", Type: "zip"}, `C:\k\a.zip`, Command{}, true},
		{"portable", Installer{File: "a.exe", Type: "portable"}, `C:\k\a.exe`, Command{}, true},
		{"msi type with an exe file", Installer{File: "a.exe", Type: "msi"}, `C:\k\a.exe`, Command{}, true},
		{"exe type with an msi file", Installer{File: "a.msi", Type: "inno"}, `C:\k\a.msi`, Command{}, true},
	}
	for _, c := range cases {
		got, err := BuildCommand(c.in, c.path)
		if (err != nil) != c.err {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if !c.err && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestSafeName(t *testing.T) {
	for _, ok := range []string{"Vendor.Alpha", "a.exe", "App 1.0.msi", "x_X64.yaml"} {
		if !SafeName(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "../x", `..\x`, "a/b", `a\b`, `C:\x.exe`, "C:x", "/abs", `\\srv\share`, "a:b", "x.", "x ", " x", "a*b", "a?b", `a"b`, "a<b", "a|b", "NUL", "con.txt", "COM1.exe", "a\x00b", "a\nb", strings.Repeat("x", 201)} {
		if SafeName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
	if ValidWingetID("Vendor.Alpha") != true || ValidWingetID("../x") || ValidWingetID("9NBLGGH4NNS1") != true {
		t.Error("ValidWingetID")
	}
}

func TestKitRoundTrip(t *testing.T) {
	fs, res := newKit(t)
	b, ok := fs.Read(filepath.Join(root, ManifestName))
	if !ok {
		t.Fatal("no kit.json written")
	}
	k, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(k, res.Kit) {
		t.Fatalf("round trip changed the kit:\n%+v\n%+v", k, res.Kit)
	}
	again, _ := Encode(k)
	if string(again) != string(b) || Fingerprint(b) != res.Fingerprint {
		t.Fatal("encoding is not stable, or the fingerprint is not of kit.json's bytes")
	}
	if k.Format != Format || k.WinForge != "v1.0.0" || k.Created != "2026-11-17T10:00:00Z" || k.Profile != "Profile" || len(k.Apps) != 2 {
		t.Fatalf("kit header: %+v", k)
	}
	a := k.Apps[0]
	if a.ID != "alpha" || a.Winget != "Vendor.Alpha" || a.Version != "1.2.3" || a.Dir != "Vendor.Alpha" || a.WinForge != "v1.0.0" || a.Created == "" {
		t.Fatalf("entry: %+v", a)
	}
	if a.Installer.Type != "inno" || a.Installer.Arch != "x64" || a.Installer.File != "Vendor_Alpha_1.2.3_X64.exe" ||
		a.Installer.ManifestFile != "Vendor_Alpha_1.2.3_X64.yaml" || a.Installer.ManifestSha256 != sum([]byte("installer of Vendor.Alpha 1.2.3")) {
		t.Fatalf("installer: %+v", a.Installer)
	}
	if len(a.Files) != 2 || a.Size != a.Files[0].Size+a.Files[1].Size {
		t.Fatalf("files/size: %+v %d", a.Files, a.Size)
	}
	for _, f := range a.Files {
		data, _ := fs.Read(filepath.Join(root, a.Dir, f.Name))
		if f.SHA256 != sum(data) || f.Size != int64(len(data)) {
			t.Fatalf("file %s recorded wrong", f.Name)
		}
	}
	if !k.Apps[1].Admin {
		t.Error("a catalog app marked admin must be admin in the kit")
	}
	if _, err := Decode([]byte(`{"format":99,"apps":[]}`)); !IsCode(err, CodeNewer) {
		t.Errorf("newer format: %v", err)
	}
	for _, bad := range []string{`not json`, `{}`, `{"format":0}`, `[]`} {
		if _, err := Decode([]byte(bad)); !IsCode(err, CodeBadKit) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestCreatePartialFailuresAndNotIncluded(t *testing.T) {
	fs := NewMemFS()
	alpha, _ := pkg("Vendor.Alpha", "1.0", "nullsoft", "exe", "")
	zip, _ := pkg("Vendor.Zipper", "2.0", "zip", "zip", "")
	noswi, _ := pkg("Vendor.NoSwitches", "3.0", "exe", "exe", "")
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{
		"Vendor.Alpha": {Files: alpha}, "Vendor.Zipper": {Files: zip}, "Vendor.NoSwitches": {Files: noswi},
		"Vendor.Fail": {Code: 0x8A150008}, "Vendor.Gamma": {Err: errors.New("winget missing")},
	}}
	var progress []Progress
	res, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1", Now: fixedNow, Progress: func(p Progress) { progress = append(progress, p) }},
		[]string{"store", "alpha", "zipper", "fail", "noswi", "gamma", "alpha", "nope"}, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Added, []string{"alpha"}) {
		t.Fatalf("added %v", res.Added)
	}
	got := map[string]NotIncluded{}
	for _, n := range res.Kit.NotIncluded {
		got[n.ID] = n
	}
	want := map[string][2]string{
		"store": {NotStore, ""}, "zipper": {NotType, "zip"}, "fail": {NotDownload, "0x8a150008"},
		"noswi": {NotSwitches, ""}, "gamma": {NotDownload, "winget missing"}, "nope": {NotUnknownApp, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("not included: %+v", got)
	}
	for id, w := range want {
		if got[id].Reason != w[0] || got[id].Detail != w[1] {
			t.Errorf("%s: %+v, want %v", id, got[id], w)
		}
	}
	// The kit stays usable and tidy: nothing of the failed apps is left, and only alpha was asked from winget once.
	if names := fs.Names(root); !reflect.DeepEqual(names, []string{"Vendor.Alpha", ManifestName}) {
		t.Fatalf("folder holds %v", names)
	}
	if rep := verify(t, fs); find(rep, "alpha").State != StateReady {
		t.Fatalf("alpha: %+v", find(rep, "alpha"))
	}
	for _, c := range dl.Calls {
		if c == "9NBLGGH4NNS1" {
			t.Fatal("a Store app was handed to winget download")
		}
	}
	last := progress[len(progress)-1]
	if last.Task != "create" || last.Done != last.Total || last.Total != 5 {
		t.Fatalf("last progress %+v", last)
	}
	sawPercent := false
	for _, p := range progress {
		sawPercent = sawPercent || p.Percent == 50
	}
	if !sawPercent {
		t.Error("winget's percent never reached the progress")
	}
}

func TestCreateCancelKeepsWhatFinished(t *testing.T) {
	fs := NewMemFS()
	a, _ := pkg("Vendor.Alpha", "1.0", "inno", "exe", "")
	b, _ := pkg("Vendor.Beta", "1.0", "msi", "msi", "")
	g, _ := pkg("Vendor.Gamma", "1.0", "inno", "exe", "")
	ctx, cancel := context.WithCancel(context.Background())
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{"Vendor.Alpha": {Files: a}, "Vendor.Beta": {Files: b}, "Vendor.Gamma": {Files: g}}}
	dl.OnCall = func(n int, id string) {
		if n == 1 {
			cancel() // stopped while beta downloads
		}
	}
	res, err := Create(ctx, Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1", Now: fixedNow}, []string{"alpha", "beta", "gamma"}, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cancelled || !reflect.DeepEqual(res.Added, []string{"alpha"}) {
		t.Fatalf("cancelled=%v added=%v", res.Cancelled, res.Added)
	}
	if len(dl.Calls) != 2 {
		t.Fatalf("downloads started after the cancel: %v", dl.Calls)
	}
	reasons := map[string]string{}
	for _, n := range res.Kit.NotIncluded {
		reasons[n.ID] = n.Reason
	}
	if reasons["beta"] != NotCancelled || reasons["gamma"] != NotCancelled {
		t.Fatalf("reasons %v", reasons)
	}
	if fs.Exists(filepath.Join(root, "Vendor.Beta")) {
		t.Fatal("the half-downloaded folder was left behind")
	}
	if rep := verify(t, fs); len(rep.Apps) != 1 || rep.Apps[0].State != StateReady {
		t.Fatalf("the kit is not usable after a cancel: %+v", rep.Apps)
	}
}

func TestPlanFreeSpaceAndFolder(t *testing.T) {
	cat := testCatalog()
	fs := NewMemFS()
	fs.Capacity = 400 << 20 // room for two estimates only
	p := Plan(context.Background(), fs, cat, []string{"alpha", "beta", "gamma", "store"}, root)
	if p.NeedBytes != 3*EstimatePerApp || !p.FreeKnown || p.FreeBytes != 400<<20 || p.Enough {
		t.Fatalf("plan %+v", p)
	}
	if len(p.Apps) != 3 || len(p.NotIncluded) != 1 || p.NotIncluded[0].Reason != NotStore {
		t.Fatalf("apps/notIncluded %+v", p)
	}
	fs.Capacity = 0
	if p := Plan(context.Background(), fs, cat, []string{"alpha"}, root); !p.Enough || p.Problem != "" {
		t.Fatalf("plenty of space: %+v", p)
	}
	// A folder holding somebody's files is never written into.
	fs.AddFile(filepath.Join(root, "notes.txt"), []byte("x"))
	if p := Plan(context.Background(), fs, cat, []string{"alpha"}, root); p.Problem != CodeFolder {
		t.Fatalf("non-empty folder: %+v", p)
	}
	for _, bad := range []string{"", "relative/dir"} {
		if p := Plan(context.Background(), fs, cat, []string{"alpha"}, bad); p.Problem != CodeFolder {
			t.Errorf("%q: %+v", bad, p)
		}
	}
	fs.AddLink(filepath.Join(os.TempDir(), "linkdir"), true)
	if p := Plan(context.Background(), fs, cat, []string{"alpha"}, filepath.Join(os.TempDir(), "linkdir")); p.Problem != CodeFolder {
		t.Fatalf("a link as the kit folder: %+v", p)
	}
}

func TestCreateStopsAppsWhenTheDriveIsFull(t *testing.T) {
	fs := NewMemFS()
	a, _ := pkg("Vendor.Alpha", "1.0", "inno", "exe", "")
	b, _ := pkg("Vendor.Beta", "1.0", "msi", "msi", "")
	// Room for the reserve plus a little: once alpha (a few hundred bytes) is written, less than the reserve is
	// free, so beta must be refused for space up front instead of failing halfway through its download.
	fs.Capacity = Reserve + 200
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{"Vendor.Alpha": {Files: a}, "Vendor.Beta": {Files: b}}}
	res, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1", Now: fixedNow}, []string{"alpha", "beta"}, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 1 {
		t.Fatalf("added %v, failed %+v", res.Added, res.Failed)
	}
	if len(res.Failed) != 1 || res.Failed[0].Reason != NotSpace {
		t.Fatalf("failed %+v", res.Failed)
	}
}

func TestCreateAddsToAnExistingKit(t *testing.T) {
	fs, first := newKit(t)
	g, _ := pkg("Vendor.Gamma", "9.9", "nullsoft", "exe", "")
	a2, _ := pkg("Vendor.Alpha", "2.0", "inno", "exe", "")
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{"Vendor.Gamma": {Files: g}, "Vendor.Alpha": {Files: a2}}}
	res, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v2", Now: fixedNow}, []string{"gamma", "alpha"}, root, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Kit.Apps) != 3 || res.Kit.Created != first.Kit.Created || res.Kit.Updated == "" || res.Kit.Profile != "" {
		t.Fatalf("merged kit: %+v", res.Kit)
	}
	for _, a := range res.Kit.Apps {
		if a.ID == "alpha" && a.Version != "2.0" {
			t.Fatalf("alpha was not replaced: %+v", a)
		}
	}
	rep := verify(t, fs)
	for _, a := range rep.Apps {
		if a.State != StateReady {
			t.Fatalf("%s: %+v", a.Entry.ID, a)
		}
	}
	// A download that fails must not remove the copy that already works.
	dl2 := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{"Vendor.Alpha": {Code: 1}}}
	res2, err := Create(context.Background(), Options{FS: fs, Dl: dl2, Cat: testCatalog(), Version: "v3", Now: fixedNow}, []string{"alpha"}, root, "")
	if err != nil || len(res2.Failed) != 1 {
		t.Fatalf("%v %+v", err, res2)
	}
	if got := find(res2.Kit.toReport(), "alpha"); got.Entry.Version != "2.0" {
		t.Fatalf("a failed refresh dropped the working app: %+v", got)
	}
}

func (k *Kit) toReport() *Report {
	r := &Report{Kit: k}
	for _, e := range k.Apps {
		r.Apps = append(r.Apps, AppReport{Entry: e})
	}
	return r
}

func TestVerifyCleanKit(t *testing.T) {
	fs, res := newKit(t)
	rep := verify(t, fs)
	if rep.Fingerprint != res.Fingerprint || len(rep.Apps) != 2 || len(rep.Unlisted) != 0 {
		t.Fatalf("%+v", rep)
	}
	for _, a := range rep.Apps {
		if a.State != StateReady || len(a.Problems) != 0 {
			t.Fatalf("%s: %+v", a.Entry.ID, a)
		}
	}
}

func TestVerifyRefusesWhatChanged(t *testing.T) {
	dir := func(id string) string { return filepath.Join(root, "Vendor."+strings.Title(id)) }
	inst := func(id, ver, ext string) string {
		return filepath.Join(dir(id), "Vendor_"+strings.Title(id)+"_"+ver+"_X64."+ext)
	}
	t.Run("tampered installer", func(t *testing.T) {
		fs, _ := newKit(t)
		p := inst("alpha", "1.2.3", "exe")
		data, _ := fs.Read(p)
		data[0] ^= 0xFF // same size, different bytes
		fs.files[key(p)] = data
		rep := verify(t, fs)
		a := find(rep, "alpha")
		if a.State != StateProblem || !reflect.DeepEqual(problemKinds(a), []string{"changed:Vendor_Alpha_1.2.3_X64.exe"}) {
			t.Fatalf("%+v", a)
		}
		if find(rep, "beta").State != StateReady {
			t.Fatal("the untouched app must stay usable")
		}
	})
	t.Run("different size", func(t *testing.T) {
		fs, _ := newKit(t)
		p := inst("alpha", "1.2.3", "exe")
		fs.AddFile(p, []byte("short"))
		if k := problemKinds(find(verify(t, fs), "alpha")); !reflect.DeepEqual(k, []string{"size:Vendor_Alpha_1.2.3_X64.exe"}) {
			t.Fatalf("%v", k)
		}
	})
	t.Run("missing file", func(t *testing.T) {
		fs, _ := newKit(t)
		_ = fs.RemoveAll(inst("beta", "4.5", "msi"))
		if k := problemKinds(find(verify(t, fs), "beta")); !reflect.DeepEqual(k, []string{"missing:Vendor_Beta_4.5_X64.msi"}) {
			t.Fatalf("%v", k)
		}
	})
	t.Run("missing folder", func(t *testing.T) {
		fs, _ := newKit(t)
		_ = fs.RemoveAll(dir("beta"))
		if k := problemKinds(find(verify(t, fs), "beta")); !reflect.DeepEqual(k, []string{"missingdir:"}) {
			t.Fatalf("%v", k)
		}
	})
	t.Run("extra file next to the installer", func(t *testing.T) {
		fs, _ := newKit(t)
		fs.AddFile(filepath.Join(dir("alpha"), "version.dll"), []byte("evil"))
		a := find(verify(t, fs), "alpha")
		if a.State != StateProblem || !reflect.DeepEqual(problemKinds(a), []string{"extra:version.dll"}) {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("extra folder inside", func(t *testing.T) {
		fs, _ := newKit(t)
		fs.AddDir(filepath.Join(dir("alpha"), "Dependencies"))
		if k := problemKinds(find(verify(t, fs), "alpha")); !reflect.DeepEqual(k, []string{"extra:Dependencies"}) {
			t.Fatalf("%v", k)
		}
	})
	t.Run("symlink as a file", func(t *testing.T) {
		fs, _ := newKit(t)
		p := inst("alpha", "1.2.3", "exe")
		_ = fs.RemoveAll(p)
		fs.AddLink(p, false)
		a := find(verify(t, fs), "alpha")
		if a.State != StateProblem || !reflect.DeepEqual(problemKinds(a), []string{"link:Vendor_Alpha_1.2.3_X64.exe"}) {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("symlink as the app folder", func(t *testing.T) {
		fs, _ := newKit(t)
		_ = fs.RemoveAll(dir("beta"))
		fs.AddLink(dir("beta"), true)
		a := find(verify(t, fs), "beta")
		if a.State != StateProblem || !reflect.DeepEqual(problemKinds(a), []string{"link:Vendor.Beta"}) {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("an unlisted folder in the root is reported, not run", func(t *testing.T) {
		fs, _ := newKit(t)
		fs.AddFile(filepath.Join(root, "Other.App", "x.exe"), []byte("x"))
		rep := verify(t, fs)
		if !reflect.DeepEqual(rep.Unlisted, []string{"Other.App"}) || find(rep, "alpha").State != StateReady {
			t.Fatalf("%+v", rep.Unlisted)
		}
	})
	t.Run("a stick pulled out while reading", func(t *testing.T) {
		fs, _ := newKit(t)
		fs.FailOpen[key(inst("alpha", "1.2.3", "exe"))] = errors.New("device not ready")
		if k := problemKinds(find(verify(t, fs), "alpha")); !reflect.DeepEqual(k, []string{"unreadable:Vendor_Alpha_1.2.3_X64.exe"}) {
			t.Fatalf("%v", k)
		}
	})
}

// edit rewrites kit.json after changing the decoded kit, the way an attacker (or a bug) could.
func edit(t *testing.T, fs *MemFS, f func(*Kit)) {
	t.Helper()
	b, _ := fs.Read(filepath.Join(root, ManifestName))
	k, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	f(k)
	nb, _ := Encode(k)
	fs.files[key(filepath.Join(root, ManifestName))] = nb
}

func TestVerifyRefusesPathTricksInKitJSON(t *testing.T) {
	bad := []struct {
		name string
		f    func(*Kit)
	}{
		{"parent folder as dir", func(k *Kit) { k.Apps[0].Dir = ".." }},
		{"relative escape in dir", func(k *Kit) { k.Apps[0].Dir = `..\Windows`; k.Apps[0].Winget = `..\Windows` }},
		{"absolute dir", func(k *Kit) { k.Apps[0].Dir = `C:\Windows`; k.Apps[0].Winget = `C:\Windows` }},
		{"dir is not the winget id", func(k *Kit) { k.Apps[0].Dir = "Vendor.Other" }},
		{"escaping file name", func(k *Kit) { k.Apps[0].Files[0].Name = `..\evil.exe`; k.Apps[0].Installer.File = `..\evil.exe` }},
		{"absolute file name", func(k *Kit) { k.Apps[0].Files[0].Name = `C:\evil.exe`; k.Apps[0].Installer.File = `C:\evil.exe` }},
		{"forward slash file name", func(k *Kit) { k.Apps[0].Files[0].Name = "sub/evil.exe"; k.Apps[0].Installer.File = "sub/evil.exe" }},
		{"installer not among the files", func(k *Kit) { k.Apps[0].Installer.File = "other.exe" }},
		{"bad hash text", func(k *Kit) { k.Apps[0].Files[0].SHA256 = "xyz" }},
		{"negative size", func(k *Kit) { k.Apps[0].Files[0].Size = -1 }},
		{"same file twice", func(k *Kit) { k.Apps[0].Files = append(k.Apps[0].Files, k.Apps[0].Files[0]) }},
		{"manifest is the installer", func(k *Kit) { k.Apps[0].Installer.ManifestFile = k.Apps[0].Installer.File }},
		{"empty id", func(k *Kit) { k.Apps[0].ID = "" }},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			fs, _ := newKit(t)
			// A file outside the kit that a path trick would reach.
			fs.AddFile(filepath.Join(os.TempDir(), "evil.exe"), []byte("evil"))
			edit(t, fs, c.f)
			rep := verify(t, fs)
			if a := rep.Apps[0]; a.State != StateProblem || a.Problems[0].Kind != ProbInvalid {
				t.Fatalf("accepted: %+v", a)
			}
			if rep.Apps[1].State != StateReady {
				t.Fatal("the other app must stay usable")
			}
		})
	}
	t.Run("same app twice", func(t *testing.T) {
		fs, _ := newKit(t)
		edit(t, fs, func(k *Kit) { k.Apps = append(k.Apps, k.Apps[0]) })
		rep := verify(t, fs)
		if rep.Apps[2].State != StateProblem || rep.Apps[2].Problems[len(rep.Apps[2].Problems)-1].Kind != ProbDuplicate {
			t.Fatalf("%+v", rep.Apps[2])
		}
	})
	t.Run("an app the catalog does not know", func(t *testing.T) {
		fs, _ := newKit(t)
		edit(t, fs, func(k *Kit) { k.Apps[0].ID = "mystery" })
		if a := verify(t, fs).Apps[0]; a.State != StateProblem || a.Problems[0].Kind != ProbUnknownApp {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("an app whose winget id is not the catalog's", func(t *testing.T) {
		fs, _ := newKit(t)
		_ = fs.RemoveAll(filepath.Join(root, "Vendor.Alpha"))
		fs.AddFile(filepath.Join(root, "Evil.Alpha", "x"), []byte("x"))
		edit(t, fs, func(k *Kit) { k.Apps[0].Winget, k.Apps[0].Dir = "Evil.Alpha", "Evil.Alpha" })
		if a := verify(t, fs).Apps[0]; a.State != StateProblem || a.Problems[0].Kind != ProbUnknownApp {
			t.Fatalf("%+v", a)
		}
	})
}

func TestVerifyChecksTheManifestHash(t *testing.T) {
	t.Run("installer and kit.json replaced together", func(t *testing.T) {
		// The attacker swaps the installer and fixes its entry in kit.json; the manifest still pins the real hash.
		fs, _ := newKit(t)
		p := filepath.Join(root, "Vendor.Alpha", "Vendor_Alpha_1.2.3_X64.exe")
		evil := []byte("a trojan")
		fs.AddFile(p, evil)
		edit(t, fs, func(k *Kit) {
			k.Apps[0].Files[0] = File{Name: filepath.Base(p), Size: int64(len(evil)), SHA256: sum(evil)}
		})
		// Files[0] is the installer: names sort exe before yaml.
		a := find(verify(t, fs), "alpha")
		if a.State != StateProblem || !reflect.DeepEqual(problemKinds(a), []string{"manifesthash:Vendor_Alpha_1.2.3_X64.exe"}) {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("kit.json claims other switches than the manifest", func(t *testing.T) {
		fs, _ := newKit(t)
		edit(t, fs, func(k *Kit) { k.Apps[0].Installer.Silent = "/S /D=C:\\Windows" })
		if k := problemKinds(find(verify(t, fs), "alpha")); !reflect.DeepEqual(k, []string{"manifestmismatch:Vendor_Alpha_1.2.3_X64.yaml"}) {
			t.Fatalf("%v", k)
		}
	})
	t.Run("kit.json claims another type", func(t *testing.T) {
		fs, _ := newKit(t)
		edit(t, fs, func(k *Kit) { k.Apps[0].Installer.Type = "burn" })
		if k := problemKinds(find(verify(t, fs), "alpha")); len(k) != 1 || !strings.HasPrefix(k[0], "manifestmismatch") {
			t.Fatalf("%v", k)
		}
	})
	t.Run("unreadable manifest", func(t *testing.T) {
		fs, _ := newKit(t)
		p := filepath.Join(root, "Vendor.Alpha", "Vendor_Alpha_1.2.3_X64.yaml")
		bad := []byte(":::")
		fs.AddFile(p, bad)
		edit(t, fs, func(k *Kit) {
			k.Apps[0].Files[1] = File{Name: filepath.Base(p), Size: int64(len(bad)), SHA256: sum(bad)}
		})
		if k := problemKinds(find(verify(t, fs), "alpha")); !reflect.DeepEqual(k, []string{"manifest:Vendor_Alpha_1.2.3_X64.yaml"}) {
			t.Fatalf("%v", k)
		}
	})
}

func TestVerifyReportsNoKit(t *testing.T) {
	fs := NewMemFS()
	fs.AddDir(root)
	if _, err := Verify(context.Background(), fs, root, nil, nil); !IsCode(err, CodeNoKit) {
		t.Fatalf("%v", err)
	}
	fs.AddFile(filepath.Join(root, ManifestName), []byte("garbage"))
	if _, err := Verify(context.Background(), fs, root, nil, nil); !IsCode(err, CodeBadKit) {
		t.Fatalf("%v", err)
	}
	fs2 := NewMemFS()
	fs2.AddLink(filepath.Join(root, ManifestName), false)
	if _, err := Verify(context.Background(), fs2, root, nil, nil); !IsCode(err, CodeBadKit) {
		t.Fatalf("a link as kit.json: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	fs3, _ := newKit(t)
	cancel()
	if _, err := Verify(ctx, fs3, root, nil, nil); !IsCode(err, CodeCancelled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestVerifyListsWhatCannotInstallOffline(t *testing.T) {
	fs := NewMemFS()
	// A kit someone made by hand (or an older WinForge) with a zip and an exe without switches.
	zipPkg, _ := pkg("Vendor.Zipper", "1.0", "zip", "zip", "")
	noswi, _ := pkg("Vendor.NoSwitches", "1.0", "exe", "exe", "")
	fs2 := fs
	for id, files := range map[string]map[string][]byte{"Vendor.Zipper": zipPkg, "Vendor.NoSwitches": noswi} {
		for n, d := range files {
			fs2.AddFile(filepath.Join(root, id, n), d)
		}
	}
	mk := func(id, winget, typ, ext string, files map[string][]byte) Entry {
		e := Entry{ID: id, Winget: winget, Name: id, Version: "1.0", Dir: winget}
		var inst, man string
		for n, d := range files {
			e.Files = append(e.Files, File{Name: n, Size: int64(len(d)), SHA256: sum(d)})
			if strings.HasSuffix(n, ".yaml") {
				man = n
			} else {
				inst = n
			}
		}
		m, _ := ParseManifest(files[man])
		e.Installer = Installer{File: inst, Type: typ, ManifestFile: man, ManifestSha256: m.Installers[0].Sha256}
		return e
	}
	k := &Kit{Format: 1, Apps: []Entry{mk("zipper", "Vendor.Zipper", "zip", "zip", zipPkg), mk("noswi", "Vendor.NoSwitches", "exe", "exe", noswi)}}
	b, _ := Encode(k)
	fs.AddFile(filepath.Join(root, ManifestName), b)
	rep := verify(t, fs)
	z, n := find(rep, "zipper"), find(rep, "noswi")
	if z.State != StateUnsupported || z.Unsupported != NotType || z.UnsupportedDetail != "zip" {
		t.Fatalf("zip: %+v", z)
	}
	if n.State != StateUnsupported || n.Unsupported != NotSwitches {
		t.Fatalf("exe: %+v", n)
	}
}
