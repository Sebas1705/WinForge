package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/offlinekit"
	"github.com/Sebas1705/WinForge/internal/system"
)

var kitRoot = filepath.Join(os.TempDir(), "kit-binding-test")

func kitPkg(id, version, typ, ext string) map[string][]byte {
	installer := []byte("installer " + id + " " + version)
	h := sha256.Sum256(installer)
	base := strings.ReplaceAll(id, ".", "_") + "_" + version + "_X64"
	y := fmt.Sprintf("PackageIdentifier: %s\nPackageVersion: %s\nInstallers:\n- Architecture: x64\n  InstallerType: %s\n  InstallerSha256: %s\nManifestType: merged\nManifestVersion: 1.28.0\n",
		id, version, typ, hex.EncodeToString(h[:]))
	return map[string][]byte{base + "." + ext: installer, base + ".yaml": []byte(y)}
}

type kitRig struct {
	a         *App
	fs        *offlinekit.MemFS
	dl        *offlinekit.FakeDownloader
	installed map[string]bool
	events    []any
	mu        sync.Mutex
	plans     []install.Plan
	sess      install.KitResolver
}

func newKitRig(t *testing.T) *kitRig {
	t.Helper()
	a := testApp(t)
	fs := offlinekit.NewMemFS()
	r := &kitRig{a: a, fs: fs, installed: map[string]bool{}}
	r.dl = &offlinekit.FakeDownloader{FS: fs, Packages: map[string]offlinekit.FakePackage{
		"Git.Git":      {Files: kitPkg("Git.Git", "2.50.1", "inno", "exe")},
		"VideoLAN.VLC": {Files: kitPkg("VideoLAN.VLC", "3.0.21", "msi", "msi")},
		"7zip.7zip":    {Files: kitPkg("7zip.7zip", "24.09", "exe", "exe")}, // an exe with no switches: cannot install offline
	}}
	a.kit.fs, a.kit.dl = fs, r.dl
	a.emitter = func(name string, data any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if name == "kit:progress" {
			r.events = append(r.events, data)
		}
	}
	a.uninst.detect = func() (map[string]system.Installed, error) {
		out := map[string]system.Installed{}
		for id := range r.installed {
			out[id] = system.Installed{ID: id, Version: "1.0"}
		}
		return out, nil
	}
	a.kit.start = func(plan install.Plan, title string, kit install.KitResolver) error {
		r.plans = append(r.plans, plan)
		r.sess = kit
		return nil
	}
	return r
}

func TestKitPrepareAndCreate(t *testing.T) {
	r := newKitRig(t)
	p := r.a.KitPrepare([]string{"git", "vlc", "7zip", "nope"}, kitRoot)
	if p.Problem != "" || len(p.Apps) != 3 || len(p.NotIncluded) != 1 || p.NotIncluded[0].Reason != offlinekit.NotUnknownApp || !p.Enough {
		t.Fatalf("%+v", p)
	}
	res, err := r.a.KitCreate([]string{"git", "vlc", "7zip"}, kitRoot, " Dev ")
	if err != nil {
		t.Fatal(err)
	}
	// 7-Zip's fake installer is an exe with no silent switches: listed as not included, not guessed.
	if !reflect.DeepEqual(res.Added, []string{"git", "vlc"}) || len(res.Failed) != 1 || res.Failed[0].ID != "7zip" || res.Failed[0].Reason != offlinekit.NotSwitches ||
		res.TotalApps != 2 || res.Cancelled || len(res.Fingerprint) != 64 || res.Folder != kitRoot {
		t.Fatalf("%+v", res)
	}
	if len(r.events) == 0 {
		t.Fatal("no progress events")
	}
	last := r.events[len(r.events)-1].(offlinekit.Progress)
	if last.Task != "create" || last.Done != last.Total {
		t.Fatalf("last progress %+v", last)
	}
}

func TestKitCreateIsOneTaskAtATimeAndCancels(t *testing.T) {
	r := newKitRig(t)
	started, release := make(chan struct{}), make(chan struct{})
	r.dl.OnCall = func(n int, id string) {
		if n == 0 {
			close(started)
			<-release
		}
	}
	done := make(chan struct {
		res KitCreateResult
		err error
	})
	go func() {
		res, err := r.a.KitCreate([]string{"git", "vlc"}, kitRoot, "")
		done <- struct {
			res KitCreateResult
			err error
		}{res, err}
	}()
	<-started
	if _, err := r.a.KitCreate([]string{"git"}, kitRoot, ""); !offlinekit.IsCode(err, offlinekit.CodeBusy) {
		t.Fatalf("a second task: %v", err)
	}
	if _, err := r.a.KitOpen(kitRoot); !offlinekit.IsCode(err, offlinekit.CodeBusy) {
		t.Fatalf("open during create: %v", err)
	}
	r.a.KitCancel()
	close(release)
	out := <-done
	if out.err != nil || !out.res.Cancelled || len(out.res.Added) != 0 {
		t.Fatalf("%+v %v", out.res, out.err)
	}
	// The task is over: a new one can start.
	if _, err := r.a.KitCreate([]string{"git"}, kitRoot, ""); err != nil {
		t.Fatalf("after the cancel: %v", err)
	}
}

func openedKit(t *testing.T) *kitRig {
	t.Helper()
	r := newKitRig(t)
	if _, err := r.a.KitCreate([]string{"git", "vlc"}, kitRoot, "Dev"); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestKitOpenListsAndVerifies(t *testing.T) {
	r := openedKit(t)
	r.installed["vlc"] = true
	l, err := r.a.KitOpen(kitRoot)
	if err != nil {
		t.Fatal(err)
	}
	if l.Folder != kitRoot || l.Profile != "Dev" || len(l.Fingerprint) != 64 || len(l.Apps) != 2 || l.NeedsAdmin || len(l.Unlisted) != 0 {
		t.Fatalf("%+v", l)
	}
	git, vlc := l.Apps[0], l.Apps[1]
	if git.ID != "git" || git.State != offlinekit.StateReady || git.Type != "inno" || git.Version != "2.50.1" || git.Installed || len(git.Problems) != 0 {
		t.Fatalf("git %+v", git)
	}
	if vlc.ID != "vlc" || !vlc.Installed || vlc.InstalledVersion != "1.0" || vlc.Type != "msi" || !vlc.Admin {
		// an msi installs for the machine unless the manifest says user scope, so it needs administrator rights
		t.Fatalf("vlc %+v", vlc)
	}
	// Tamper with one installer: it is reported and the other stays usable.
	p := filepath.Join(kitRoot, "Git.Git", "Git_Git_2.50.1_X64.exe")
	b, _ := r.fs.Read(p)
	b[0] ^= 1
	r.fs.AddFile(p+".tmp", nil)
	_ = r.fs.RemoveAll(p + ".tmp")
	r.fs.AddFile(p, b)
	l, err = r.a.KitOpen(kitRoot)
	if err != nil {
		t.Fatal(err)
	}
	if l.Apps[0].State != offlinekit.StateProblem || len(l.Apps[0].Problems) != 1 || l.Apps[0].Problems[0].Kind != offlinekit.ProbChanged || l.Apps[1].State != offlinekit.StateReady {
		t.Fatalf("%+v", l.Apps)
	}
	if _, err := r.a.KitPlan([]string{"git"}); !offlinekit.IsCode(err, offlinekit.CodeNotListed) {
		t.Fatalf("a tampered app was planned: %v", err)
	}
	if _, err := r.a.KitOpen(filepath.Join(os.TempDir(), "no-such-kit")); !offlinekit.IsCode(err, offlinekit.CodeNoKit) {
		t.Fatalf("no kit: %v", err)
	}
	// Opening a folder with no kit forgets the previous one: nothing can be installed from a kit that is not open.
	if _, err := r.a.KitPlan([]string{"vlc"}); !offlinekit.IsCode(err, offlinekit.CodeNotOpen) {
		t.Fatalf("after a failed open: %v", err)
	}
}

func TestKitInstallOnlyTakesIDsTheOpenedKitOffered(t *testing.T) {
	r := openedKit(t)
	if _, err := r.a.KitPlan([]string{"git"}); !offlinekit.IsCode(err, offlinekit.CodeNotOpen) {
		t.Fatalf("before opening: %v", err)
	}
	if err := r.a.KitInstall([]string{"git"}); !offlinekit.IsCode(err, offlinekit.CodeNotOpen) {
		t.Fatalf("install before opening: %v", err)
	}
	if _, err := r.a.KitOpen(kitRoot); err != nil {
		t.Fatal(err)
	}
	for name, ids := range map[string][]string{"not in the kit": {"7zip"}, "made up": {"../../evil"}, "one good one bad": {"git", "7zip"}} {
		if err := r.a.KitInstall(ids); !offlinekit.IsCode(err, offlinekit.CodeNotListed) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(r.plans) != 0 {
		t.Fatal("a refused install started a run")
	}
	if err := r.a.KitInstall(nil); !offlinekit.IsCode(err, offlinekit.CodeNothing) {
		t.Errorf("nothing ticked: %v", err)
	}
	plan, err := r.a.KitPlan([]string{"vlc", "git", "git"})
	if err != nil || len(plan.Steps) != 2 || plan.Steps[0].ID != "git" || plan.Steps[0].Kind != install.StepKit || !plan.NeedsAdmin {
		t.Fatalf("%+v %v", plan, err)
	}
	// Everything ticked is already installed: nothing to run.
	r.installed["git"], r.installed["vlc"] = true, true
	if err := r.a.KitInstall([]string{"git", "vlc"}); !offlinekit.IsCode(err, offlinekit.CodeNothing) {
		t.Fatalf("all installed: %v", err)
	}
	delete(r.installed, "git")
	if err := r.a.KitInstall([]string{"git", "vlc"}); err != nil {
		t.Fatal(err)
	}
	if len(r.plans) != 1 || len(r.plans[0].Steps) != 1 || r.plans[0].Steps[0].ID != "git" || !reflect.DeepEqual(r.plans[0].AlreadyInstalled, []string{"vlc"}) {
		t.Fatalf("%+v", r.plans)
	}
}

type recordingExec struct {
	calls []string
	code  int
}

func (e *recordingExec) Run(_ context.Context, name string, args []string, _ func(string)) (int, error) {
	e.calls = append(e.calls, filepath.Base(name)+" "+strings.Join(args, " "))
	return e.code, nil
}

func TestKitInstallRunsThroughTheRunnerAndVerifies(t *testing.T) {
	r := openedKit(t)
	if _, err := r.a.KitOpen(kitRoot); err != nil {
		t.Fatal(err)
	}
	if err := r.a.KitInstall([]string{"git", "vlc"}); err != nil {
		t.Fatal(err)
	}
	ex := &recordingExec{code: 3010}
	var events []install.Event
	failed := install.RunWith(context.Background(), r.a.cat, r.plans[0], ex, r.sess, func(e install.Event) { events = append(events, e) })
	if len(failed) != 0 || len(ex.calls) != 2 {
		t.Fatalf("failed=%v calls=%v", failed, ex.calls)
	}
	if ex.calls[0] != "Git_Git_2.50.1_X64.exe /SP- /VERYSILENT /SUPPRESSMSGBOXES /NORESTART" || !strings.HasPrefix(ex.calls[1], "msiexec /i ") {
		t.Fatalf("%v", ex.calls)
	}
	reboots := 0
	for _, e := range events {
		if e.Status == install.StatusOK && e.Reason == install.ReasonReboot {
			reboots++
		}
	}
	if reboots != 2 {
		t.Fatalf("a restart code (3010) must be reported on each step: %+v", events)
	}
	// Re-detection: git shows up, vlc does not.
	r.installed["git"] = true
	v, err := r.a.KitVerify([]string{"git", "vlc", "7zip", "../x"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Installed, []KitRef{{ID: "git", Name: "Git"}}) || !reflect.DeepEqual(v.Missing, []KitRef{{ID: "vlc", Name: "VLC"}}) {
		t.Fatalf("%+v", v)
	}
}

func TestKitVerifyNeedsAnOpenKit(t *testing.T) {
	r := newKitRig(t)
	if _, err := r.a.KitVerify([]string{"git"}); !offlinekit.IsCode(err, offlinekit.CodeNotOpen) {
		t.Fatalf("%v", err)
	}
}

// A kit step in a plan that did not come from KitInstall (a pending run read from a file, say) runs nothing.
func TestKitStepsAreNeverResumedFromPending(t *testing.T) {
	a := testApp(t)
	if a.pending == nil {
		t.Skip("no pending store in this test app")
	}
	a.savePending(install.Pending{Title: "kit", Steps: []install.Step{{Kind: install.StepKit, ID: "git"}, {Kind: install.StepApp, ID: "vlc"}}})
	p := a.livePending(map[string]system.Installed{})
	if p == nil || len(p.Steps) != 1 || p.Steps[0].ID != "vlc" {
		t.Fatalf("%+v", p)
	}
}
