package offlinekit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
)

// fakeExec records what would run and answers with canned exit codes. Nothing real is ever started.
type fakeExec struct {
	mu    sync.Mutex
	calls []call
	code  map[string]int // by program, or by the installer's file name for an exe
	err   error
	// during runs while the "installer" is running, with the program's own path.
	during func(exe string)
}

type call struct {
	Name string
	Args []string
}

func (f *fakeExec) Run(ctx context.Context, name string, args []string, onLine func(string)) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, call{name, append([]string(nil), args...)})
	f.mu.Unlock()
	if f.during != nil {
		f.during(name)
	}
	if f.err != nil {
		return -1, f.err
	}
	onLine("running")
	if c, ok := f.code[filepath.Base(name)]; ok {
		return c, nil
	}
	return f.code[name], nil
}

func selection(t *testing.T, fs *MemFS, ids ...string) (Selection, *Report) {
	t.Helper()
	rep := verify(t, fs)
	sel, err := BuildPlan(testCatalog(), rep, ids, nil)
	if err != nil {
		t.Fatal(err)
	}
	return sel, rep
}

func TestBuildPlan(t *testing.T) {
	fs, _ := newKit(t)
	rep := verify(t, fs)
	cat := testCatalog()
	sel, err := BuildPlan(cat, rep, []string{"beta", "alpha"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The kit's order (dependency order of the profile), not the order of the ticks.
	want := []install.Step{
		{Kind: install.StepKit, ID: "alpha", Name: "Alpha", Version: "1.2.3"},
		{Kind: install.StepKit, ID: "beta", Name: "Beta", Version: "4.5", Admin: true},
	}
	if !reflect.DeepEqual(sel.Plan.Steps, want) || !sel.Plan.NeedsAdmin || len(sel.Apps) != 2 {
		t.Fatalf("plan %+v", sel.Plan)
	}
	if !install.HasKitSteps(sel.Plan) || install.HasKitSteps(install.Plan{Steps: []install.Step{{Kind: install.StepApp}}}) {
		t.Fatal("HasKitSteps")
	}
	// Already installed: reported, no step, and no need for admin because of it.
	sel, err = BuildPlan(cat, rep, []string{"alpha", "beta"}, map[string]system.Installed{"beta": {ID: "beta"}})
	if err != nil || len(sel.Plan.Steps) != 1 || sel.Plan.NeedsAdmin || !reflect.DeepEqual(sel.Plan.AlreadyInstalled, []string{"beta"}) {
		t.Fatalf("%v %+v", err, sel.Plan)
	}
	for name, ids := range map[string][]string{"unknown id": {"mystery"}, "an app of the catalog that is not in the kit": {"gamma"}, "one good and one bad": {"alpha", "gamma"}} {
		if _, err := BuildPlan(cat, rep, ids, nil); !IsCode(err, CodeNotListed) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := BuildPlan(cat, rep, nil, nil); !IsCode(err, CodeNothing) {
		t.Errorf("nothing: %v", err)
	}
	if _, err := BuildPlan(cat, nil, []string{"alpha"}, nil); !IsCode(err, CodeNotOpen) {
		t.Errorf("not open: %v", err)
	}
	// An app that failed verification can never be planned, even if it is named.
	_ = fs.RemoveAll(filepath.Join(root, "Vendor.Beta"))
	rep = verify(t, fs)
	if _, err := BuildPlan(cat, rep, []string{"beta"}, nil); !IsCode(err, CodeNotListed) {
		t.Errorf("a broken app was planned: %v", err)
	}
}

func TestRunKitStepsThroughTheRunner(t *testing.T) {
	fs, _ := newKit(t)
	sel, _ := selection(t, fs, "alpha", "beta")
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ex := &fakeExec{code: map[string]int{}}
	var events []install.Event
	failed := install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(e install.Event) { events = append(events, e) })
	if len(failed) != 0 {
		t.Fatalf("failed %v: %+v", failed, events)
	}
	if len(ex.calls) != 2 {
		t.Fatalf("calls %+v", ex.calls)
	}
	// alpha: an exe run from the private copy, never from the kit folder, with inno's silent switches as separate args.
	c := ex.calls[0]
	if strings.HasPrefix(c.Name, root) || filepath.Base(c.Name) != "Vendor_Alpha_1.2.3_X64.exe" ||
		!reflect.DeepEqual(c.Args, []string{"/SP-", "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}) {
		t.Fatalf("alpha: %+v", c)
	}
	m := ex.calls[1]
	if m.Name != "msiexec" || len(m.Args) != 4 || m.Args[0] != "/i" || m.Args[2] != "/qn" || m.Args[3] != "/norestart" ||
		strings.HasPrefix(m.Args[1], root) || filepath.Base(m.Args[1]) != "Vendor_Beta_4.5_X64.msi" {
		t.Fatalf("beta: %+v", m)
	}
	var oks int
	for _, e := range events {
		if e.Status == install.StatusOK && e.Reason == "" {
			oks++
		}
	}
	if oks != 2 {
		t.Fatalf("events %+v", events)
	}
	// The private copies are gone once each step is over.
	if names := fs.Names(s.tmp); len(names) != 0 {
		t.Fatalf("copies left behind: %v", names)
	}
}

func TestRunKitExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		code   int
		ok     bool
		reason string
	}{
		{"success", 0, true, ""},
		{"restart needed", 3010, true, install.ReasonReboot},
		{"restart started", 1641, true, install.ReasonReboot},
		{"elevation required", 740, false, install.ReasonAdmin},
		{"access denied", 5, false, install.ReasonAdmin},
		{"msi generic failure", 1603, false, install.ReasonAdmin},
		{"another installation", 1618, false, install.ReasonBusy},
		{"user cancelled", 1602, false, install.ReasonCancelled},
		{"no disk space", 112, false, install.ReasonDisk},
		{"unknown failure", 9, false, install.ReasonInstaller},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs, _ := newKit(t)
			sel, _ := selection(t, fs, "alpha")
			s := NewSession(fs, root, sel.Apps)
			defer s.Close()
			ex := &fakeExec{code: map[string]int{"Vendor_Alpha_1.2.3_X64.exe": c.code}}
			var last install.Event
			failed := install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(e install.Event) { last = e })
			if c.ok {
				if len(failed) != 0 || last.Status != install.StatusOK || last.Reason != c.reason {
					t.Fatalf("%+v failed=%v", last, failed)
				}
				return
			}
			if !reflect.DeepEqual(failed, []string{"alpha"}) || last.Status != install.StatusFailed || last.Reason != c.reason {
				t.Fatalf("%+v failed=%v", last, failed)
			}
		})
	}
}

func TestRunKitManifestSuccessCodes(t *testing.T) {
	fs := NewMemFS()
	files, _ := pkg("Vendor.Alpha", "1.0", "nullsoft", "exe", "  InstallerSuccessCodes:\n  - 1234")
	fs.AddFile(filepath.Join(root, ManifestName), nil)
	dl := &FakeDownloader{FS: fs, Packages: map[string]FakePackage{"Vendor.Alpha": {Files: files}}}
	if _, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1", Now: fixedNow}, []string{"alpha"}, root, ""); err == nil {
		t.Fatal("an empty kit.json must not be accepted as an existing kit")
	}
	_ = fs.RemoveAll(filepath.Join(root, ManifestName))
	if _, err := Create(context.Background(), Options{FS: fs, Dl: dl, Cat: testCatalog(), Version: "v1", Now: fixedNow}, []string{"alpha"}, root, ""); err != nil {
		t.Fatal(err)
	}
	sel, _ := selection(t, fs, "alpha")
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ex := &fakeExec{code: map[string]int{"Vendor_Alpha_1.0_X64.exe": 1234}}
	if failed := install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(install.Event) {}); len(failed) != 0 {
		t.Fatalf("a code the manifest calls success failed: %v", failed)
	}
	if ex.calls[0].Args[0] != "/S" {
		t.Fatalf("nullsoft default switch: %+v", ex.calls[0])
	}
}

func TestSessionRefusesAFileChangedAfterTheKitWasOpened(t *testing.T) {
	fs, _ := newKit(t)
	sel, _ := selection(t, fs, "alpha", "beta")
	// Verification passed; now the stick is tampered with before the install starts.
	p := filepath.Join(root, "Vendor.Alpha", "Vendor_Alpha_1.2.3_X64.exe")
	data, _ := fs.Read(p)
	data[3] ^= 0x55
	fs.files[key(p)] = data
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ex := &fakeExec{code: map[string]int{}}
	var reasons = map[string]string{}
	failed := install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(e install.Event) {
		if e.Status == install.StatusFailed {
			reasons[e.Step.ID] = e.Reason
		}
	})
	if !reflect.DeepEqual(failed, []string{"alpha"}) || reasons["alpha"] != install.ReasonKitHash {
		t.Fatalf("failed=%v reasons=%v", failed, reasons)
	}
	if len(ex.calls) != 1 || ex.calls[0].Name != "msiexec" {
		t.Fatalf("the changed installer was run, or the good one was not: %+v", ex.calls)
	}
}

func TestSessionNeverRunsWhatTheKitDoesNotName(t *testing.T) {
	fs, _ := newKit(t)
	sel, _ := selection(t, fs, "alpha")
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ex := &fakeExec{}
	// A step for an app that was not selected, for a made-up one, and for a recipe-like id.
	plan := install.Plan{Steps: []install.Step{
		{Kind: install.StepKit, ID: "beta"}, {Kind: install.StepKit, ID: "../../evil"}, {Kind: install.StepKit, ID: ""},
	}}
	var reasons []string
	failed := install.RunWith(context.Background(), testCatalog(), plan, ex, s, func(e install.Event) {
		if e.Status == install.StatusFailed {
			reasons = append(reasons, e.Reason)
		}
	})
	if len(failed) != 3 || len(ex.calls) != 0 {
		t.Fatalf("failed=%v calls=%+v", failed, ex.calls)
	}
	for _, r := range reasons {
		if r != install.ReasonKitClosed {
			t.Fatalf("reasons %v", reasons)
		}
	}
	// And without any resolver, a kit step in a plan read from a file does nothing.
	failed = install.RunWith(context.Background(), testCatalog(), install.Plan{Steps: []install.Step{{Kind: install.StepKit, ID: "alpha"}}}, ex, nil, func(install.Event) {})
	if len(failed) != 1 || len(ex.calls) != 0 {
		t.Fatalf("nil resolver: %v %+v", failed, ex.calls)
	}
	failed = install.Run(context.Background(), testCatalog(), install.Plan{Steps: []install.Step{{Kind: install.StepKit, ID: "alpha"}}}, ex, func(install.Event) {})
	if len(failed) != 1 || len(ex.calls) != 0 {
		t.Fatalf("Run: %v %+v", failed, ex.calls)
	}
}

func TestSessionStopsOnAnUnreadableStickAndAFullDisk(t *testing.T) {
	t.Run("stick pulled out", func(t *testing.T) {
		fs, _ := newKit(t)
		sel, _ := selection(t, fs, "alpha")
		fs.FailOpen[key(filepath.Join(root, "Vendor.Alpha", "Vendor_Alpha_1.2.3_X64.exe"))] = errors.New("device not ready")
		s := NewSession(fs, root, sel.Apps)
		defer s.Close()
		var reason string
		install.RunWith(context.Background(), testCatalog(), sel.Plan, &fakeExec{}, s, func(e install.Event) {
			if e.Status == install.StatusFailed {
				reason = e.Reason
			}
		})
		if reason != install.ReasonKitFile {
			t.Fatalf("reason %q", reason)
		}
	})
	t.Run("link planted in the app folder", func(t *testing.T) {
		fs, _ := newKit(t)
		sel, _ := selection(t, fs, "alpha")
		p := filepath.Join(root, "Vendor.Alpha", "Vendor_Alpha_1.2.3_X64.exe")
		_ = fs.RemoveAll(p)
		fs.AddLink(p, false)
		s := NewSession(fs, root, sel.Apps)
		defer s.Close()
		ex := &fakeExec{}
		var reason string
		install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(e install.Event) {
			if e.Status == install.StatusFailed {
				reason = e.Reason
			}
		})
		if reason != install.ReasonKitHash || len(ex.calls) != 0 {
			t.Fatalf("reason %q calls %v", reason, ex.calls)
		}
	})
	t.Run("no room for the private copy", func(t *testing.T) {
		fs, _ := newKit(t)
		sel, _ := selection(t, fs, "alpha")
		fs.Capacity = 1 // used > capacity: free space is negative
		s := NewSession(fs, root, sel.Apps)
		defer s.Close()
		ex := &fakeExec{}
		var reason string
		install.RunWith(context.Background(), testCatalog(), sel.Plan, ex, s, func(e install.Event) {
			if e.Status == install.StatusFailed {
				reason = e.Reason
			}
		})
		if reason != install.ReasonDisk || len(ex.calls) != 0 {
			t.Fatalf("reason %q calls %v", reason, ex.calls)
		}
	})
}

func TestRunKitSkipsWhatDependsOnAFailedApp(t *testing.T) {
	fs, _ := newKit(t)
	sel, _ := selection(t, fs, "alpha", "beta")
	cat := testCatalog()
	cat.Apps["beta"].Requires = []string{"alpha"}
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ex := &fakeExec{code: map[string]int{"Vendor_Alpha_1.2.3_X64.exe": 9}}
	var skipped []string
	failed := install.RunWith(context.Background(), cat, sel.Plan, ex, s, func(e install.Event) {
		if e.Status == install.StatusSkipped {
			skipped = append(skipped, e.Step.ID)
		}
	})
	if !reflect.DeepEqual(failed, []string{"alpha"}) || !reflect.DeepEqual(skipped, []string{"beta"}) || len(ex.calls) != 1 {
		t.Fatalf("failed=%v skipped=%v calls=%+v", failed, skipped, ex.calls)
	}
}

func TestRunKitCancel(t *testing.T) {
	fs, _ := newKit(t)
	sel, _ := selection(t, fs, "alpha", "beta")
	s := NewSession(fs, root, sel.Apps)
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	ex := &fakeExec{during: func(string) { cancel() }}
	var skipped []string
	install.RunWith(ctx, testCatalog(), sel.Plan, ex, s, func(e install.Event) {
		if e.Status == install.StatusSkipped {
			skipped = append(skipped, e.Step.ID)
		}
	})
	if len(ex.calls) != 1 || !reflect.DeepEqual(skipped, []string{"beta"}) {
		t.Fatalf("calls %+v skipped %v", ex.calls, skipped)
	}
}

func TestOSFSOnARealFolder(t *testing.T) {
	dir := t.TempDir()
	fs := OSFS{}
	sub := filepath.Join(dir, "App.One")
	if err := fs.MkdirAll(sub); err != nil {
		t.Fatal(err)
	}
	w, err := fs.Create(filepath.Join(sub, "a.exe"))
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
	w.Close()
	if _, err := fs.Create(filepath.Join(sub, "a.exe")); err == nil {
		t.Fatal("Create must not overwrite")
	}
	size, sh, err := hashFile(context.Background(), fs, filepath.Join(sub, "a.exe"))
	if err != nil || size != 5 || sh != sum([]byte("hello")) {
		t.Fatalf("%d %s %v", size, sh, err)
	}
	ents, err := fs.ReadDir(sub)
	if err != nil || len(ents) != 1 || ents[0].Name != "a.exe" || ents[0].Link || ents[0].Dir || ents[0].Size != 5 {
		t.Fatalf("%+v %v", ents, err)
	}
	di, err := fs.Lstat(sub)
	if err != nil || !di.Dir || di.Link {
		t.Fatalf("%+v %v", di, err)
	}
	// A symbolic link is a link, never a plain file or folder (skipped when the OS does not allow creating one).
	if err := os.Symlink(filepath.Join(sub, "a.exe"), filepath.Join(sub, "ln.exe")); err == nil {
		if li, err := fs.Lstat(filepath.Join(sub, "ln.exe")); err != nil || !li.Link {
			t.Fatalf("symlink file: %+v %v", li, err)
		}
	}
	if err := os.Symlink(sub, filepath.Join(dir, "lnk")); err == nil {
		if li, err := fs.Lstat(filepath.Join(dir, "lnk")); err != nil || !li.Link || li.Dir {
			t.Fatalf("symlink dir: %+v %v", li, err)
		}
	}
	tmp, err := fs.MkdirTemp("winforge-kit-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer fs.RemoveAll(tmp)
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal(err)
	}
}
