package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/cliout"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

func TestUsageListsTheUpdatesCommand(t *testing.T) {
	if !strings.Contains(usage, "winforge-cli updates [--mode notify|apply] [--scheduled]") || !strings.Contains(usage, "updates --help") {
		t.Errorf("the usage text does not describe `updates`:\n%s", usage)
	}
}

func TestUpdatesIsAnsweredBeforeTheCatalogLoads(t *testing.T) {
	// run() hands `updates ...` to the shared runner; its --help is the long text and needs nothing from this PC.
	if !sched.IsUpdatesCommand([]string{"winforge-cli", "updates", "--help"}) {
		t.Fatal("`winforge-cli updates` must reach the shared runner")
	}
	if sched.IsUpdatesCommand([]string{"winforge-cli", "health", "updates"}) {
		t.Fatal("`health updates` is another command")
	}
}

// ---- fakes ------------------------------------------------------------------------------------

// fakeExec records winget/PowerShell calls and answers with a scripted exit code per command line.
type fakeExec struct {
	mu    sync.Mutex
	calls []string
	exit  func(name string, args []string) int
	block bool // wait for the context instead of finishing
}

func (f *fakeExec) Run(ctx context.Context, name string, args []string, onLine func(string)) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	onLine("working")
	if f.exit != nil {
		return f.exit(name, args), nil
	}
	return 0, nil
}

type rig struct {
	t       *testing.T
	d       deps
	cat     *catalog.Catalog
	ex      *fakeExec
	elev    bool
	snapErr error
	inv     func(call int) map[string]string // winget inventory per snapshot call
	snaps   int
	lock    string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, cat: cat, ex: &fakeExec{}, lock: filepath.Join(t.TempDir(), "run.lock")}
	r.d = deps{
		catalog: func() (*catalog.Catalog, error) { return cat, nil },
		snapshot: func(context.Context) (system.Inventory, error) {
			r.snaps++
			w := map[string]string{}
			if r.inv != nil {
				w = r.inv(r.snaps)
			}
			return system.Inventory{Winget: w, LookPath: func(string) bool { return false }, Exists: func(string) bool { return false }}, r.snapErr
		},
		exec:     r.ex,
		elevated: func() bool { return r.elev },
		lockPath: func() (string, error) { return r.lock, nil },
		readFile: os.ReadFile,
		health: func(context.Context, bool) (*healthOut, error) {
			return &healthOut{Machine: machineJSON{"Acme", "Box", "Windows 11", "CPU", 16},
				Findings: []health.Finding{{Key: "disk.low", Group: "storage", Severity: "warn", Params: map[string]string{"free": "3"}}}}, nil
		},
		updates: func(args []string, o cliout.Options, out, errw io.Writer) int {
			return sched.RunCLIWith(args, o, out, errw, sched.Deps{Runner: &sched.Runner{}})
		},
	}
	return r
}

func (r *rig) run(args ...string) (code int, out, errw string) {
	var o, e bytes.Buffer
	code = run(context.Background(), args, &o, &e, r.d)
	return code, o.String(), e.String()
}

// profileWith finds a profile whose plan on an empty PC has steps, with or without admin steps.
func (r *rig) profileWith(admin bool) string {
	ids := make([]string, 0)
	for id := range r.cat.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		res, err := r.cat.Resolve(id)
		if err != nil {
			continue
		}
		p := install.BuildPlan(r.cat, res, map[string]system.Installed{})
		if len(p.Steps) > 0 && len(p.Steps) <= 400 && p.NeedsAdmin == admin {
			return id
		}
	}
	r.t.Skipf("no profile with admin=%v", admin)
	return ""
}

// doc parses stdout as exactly one JSON document and checks the contract of the envelope.
func doc(t *testing.T, cmd string, code int, out string) map[string]any {
	t.Helper()
	if strings.HasPrefix(out, "\ufeff") {
		t.Error("BOM")
	}
	if strings.Contains(out, "\x1b") {
		t.Error("ANSI escape in stdout")
	}
	if !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 {
		t.Errorf("stdout must be one line ending in a newline: %q", out)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if dec.More() {
		t.Error("more than one document")
	}
	if m["schema"] != float64(cliout.SchemaVersion) || m["command"] != cmd || m["exitCode"] != float64(code) {
		t.Errorf("envelope %v (want command %s code %d)", m, cmd, code)
	}
	if m["ok"] != exitcode.Succeeded(code) {
		t.Errorf("ok = %v for code %d", m["ok"], code)
	}
	if _, has := m["error"]; !has {
		t.Error("error member missing (null when none)")
	}
	if exitcode.Succeeded(code) != (m["error"] == nil) {
		t.Errorf("error = %v for code %d", m["error"], code)
	}
	return m
}

// ---- help and exit codes ----------------------------------------------------------------------

func TestEveryHelpListsEveryExitCodeAndTheGlobalOptions(t *testing.T) {
	r := newRig(t)
	for _, cmd := range commands {
		code, out, _ := r.run(cmd, "--help")
		if code != exitcode.OK {
			t.Errorf("%s --help: %d", cmd, code)
		}
		for _, e := range exitcode.Table {
			if !regexp.MustCompile(fmt.Sprintf(`(?m)^  %d  \S`, e.Code)).MatchString(out) {
				t.Errorf("`%s --help` does not list exit code %d:\n%s", cmd, e.Code, out)
			}
		}
		for _, w := range []string{"--json", "--quiet", "--no-color", "--yes", "--non-interactive", "--timeout", "Exit codes"} {
			if !strings.Contains(out, w) {
				t.Errorf("`%s --help` does not mention %s", cmd, w)
			}
		}
		if !strings.HasPrefix(out, "usage: winforge-cli "+cmd) {
			t.Errorf("%s: help starts %q", cmd, out[:30])
		}
	}
}

func TestExitCodeTableIsStable(t *testing.T) {
	want := map[int]string{0: "ok", 1: "failures", 2: "usage", 3: "busy", 4: "prerequisite", 5: "data", 6: "timeout", 7: "admin", 8: "reboot"}
	if len(exitcode.Table) != len(want) {
		t.Fatalf("table has %d entries", len(exitcode.Table))
	}
	for _, e := range exitcode.Table {
		if want[e.Code] != e.Name || e.Meaning == "" {
			t.Errorf("%+v", e)
		}
	}
	// the numbers `updates` documented before stay what they were
	if sched.ExitOK != 0 || sched.ExitFailures != 1 || sched.ExitUsage != 2 || sched.ExitBusy != 3 ||
		sched.ExitCheck != 4 || sched.ExitSettings != 5 || sched.ExitTimeout != 6 {
		t.Error("an exit code of `updates` changed")
	}
}

func TestSplitReadsGlobalOptionsAnywhere(t *testing.T) {
	rest, o, err := cliout.Split([]string{"--json", "plan", "dev", "--quiet", "--timeout", "90s", "--yes", "--no-color"})
	if err != nil || strings.Join(rest, " ") != "plan dev" || !o.JSON || !o.Quiet || !o.NonInteractive || !o.NoColor || o.Timeout != 90*time.Second {
		t.Fatalf("%v %v %+v", rest, err, o)
	}
	_, o, _ = cliout.Split([]string{"apply", "x", "--non-interactive", "--timeout=2m"})
	if !o.NonInteractive || o.Timeout != 2*time.Minute {
		t.Errorf("%+v", o)
	}
	for _, bad := range [][]string{{"--timeout"}, {"--timeout", "soon"}, {"--timeout=0"}, {"--timeout=-5s"}} {
		if _, _, err := cliout.Split(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestUsageMistakes(t *testing.T) {
	r := newRig(t)
	for _, args := range [][]string{{}, {"nope"}, {"plan"}, {"plan", "a", "b"}, {"plan", "no-such-profile"}, {"scan", "x"}, {"scan", "--nope"}, {"export"}, {"export", "a", "b"},
		{"health", "x"}, {"check"}, {"--timeout"}, {"profiles", "x"}} {
		code, out, errw := r.run(args...)
		if code != exitcode.Usage || out != "" || errw == "" {
			t.Errorf("%v: %d %q %q", args, code, out, errw)
		}
		code, out, _ = r.run(append([]string{"--json"}, args...)...)
		if code != exitcode.Usage {
			t.Errorf("--json %v: %d", args, code)
			continue
		}
		cmd := ""
		if len(args) > 0 && contains(commands, args[0]) {
			cmd = args[0]
		}
		if len(args) > 0 && args[0] == "--timeout" {
			continue // the flag itself is broken: the document has no command
		}
		m := doc(t, cmd, code, out)
		if e := m["error"].(map[string]any); e["code"] != "usage" || e["message"] == "" {
			t.Errorf("%v: %v", args, e)
		}
	}
	if len(r.ex.calls) != 0 {
		t.Error("a usage mistake started something")
	}
}

// ---- JSON documents ---------------------------------------------------------------------------

func TestJSONDocumentsOfTheReadOnlyCommands(t *testing.T) {
	r := newRig(t)
	r.inv = func(int) map[string]string { return map[string]string{"git.git": "2.50.0"} }
	profile := r.profileWith(false)

	code, out, errw := r.run("--json", "profiles")
	m := doc(t, "profiles", code, out)
	if ps := m["profiles"].([]any); len(ps) != len(r.cat.Profiles) || errw != "" {
		t.Errorf("%d profiles, stderr %q", len(ps), errw)
	}

	code, out, _ = r.run("scan", "--json")
	m = doc(t, "scan", code, out)
	if m["catalogCount"] != float64(len(r.cat.Apps)) || m["warnings"] == nil {
		t.Errorf("%v", m)
	}
	for _, it := range m["installed"].([]any) {
		for _, k := range []string{"id", "name", "version", "sources"} {
			if _, ok := it.(map[string]any)[k]; !ok {
				t.Errorf("installed item lacks %s", k)
			}
		}
	}

	code, out, _ = r.run("plan", profile, "--json")
	m = doc(t, "plan", code, out)
	if m["profile"] != profile || len(m["steps"].([]any)) == 0 || m["alreadyInstalled"] == nil || m["skippedRecipes"] == nil {
		t.Errorf("%v", m)
	}
	step := m["steps"].([]any)[0].(map[string]any)
	if step["kind"] == nil || step["id"] == nil || step["name"] == nil {
		t.Errorf("%v", step)
	}

	code, out, _ = r.run("script", profile, "--json")
	m = doc(t, "script", code, out)
	if s, _ := m["script"].(string); !strings.Contains(s, "winget") {
		t.Errorf("%v", m)
	}

	code, out, _ = r.run("configure", profile, "--json")
	m = doc(t, "configure", code, out)
	if m["format"] != "winget-configure" || m["content"] == "" {
		t.Errorf("%v", m)
	}

	code, out, _ = r.run("export", "mine", "pin", "--json")
	m = doc(t, "export", code, out)
	if p, ok := m["profile"].(map[string]any); !ok || p["profile"].(map[string]any)["id"] != "mine" {
		t.Errorf("%v", m)
	}

	code, out, _ = r.run("health", "--json")
	m = doc(t, "health", code, out)
	if m["total"] != float64(1) || m["machine"].(map[string]any)["model"] != "Box" || len(m["findings"].([]any)) != 1 {
		t.Errorf("%v", m)
	}
}

func TestHumanOutputIsUnchangedWithoutJSON(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	code, out, errw := r.run("plan", profile)
	if code != 0 || errw != "" || !regexp.MustCompile(`^\d+ already installed, \d+ steps\n  (app|recipe)`).MatchString(out) {
		t.Errorf("%d %q %q", code, out, errw)
	}
	code, out, _ = r.run("scan")
	if code != 0 || !regexp.MustCompile(`\n\d+ of \d+ catalog apps installed\n$`).MatchString(out) || strings.Contains(out, "{") {
		t.Errorf("%d %q", code, out)
	}
	code, out, _ = r.run("profiles")
	if code != 0 || !strings.Contains(out, profile) {
		t.Errorf("%d %q", code, out)
	}
	code, out, _ = r.run("health")
	if code != 0 || !strings.HasPrefix(out, "Acme Box | Windows 11 | CPU (16 GB RAM)\n[warn   ] disk.low") || !strings.HasSuffix(out, "\n0 of 1 checks fine\n") {
		t.Errorf("%d %q", code, out)
	}
	code, out, _ = r.run("--no-color", "--yes", "plan", profile)
	if code != 0 || strings.Contains(out, "\x1b") {
		t.Errorf("%d", code)
	}
}

func TestCheckCommand(t *testing.T) {
	r := newRig(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	exported := func() string { _, out, _ := r.run("export", "mine"); return out }()
	if err := os.WriteFile(good, []byte(exported), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := r.run("check", good, "--json")
	m := doc(t, "check", code, out)
	if m["file"] != good || m["unknownApps"] == nil || m["unknownRecipes"] == nil {
		t.Errorf("%v", m)
	}
	code, out, _ = r.run("check", good)
	if code != 0 || !strings.HasPrefix(out, "ok: ") {
		t.Errorf("%d %q", code, out)
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte("{not json"), 0o644)
	for _, f := range []string{bad, filepath.Join(dir, "missing.json")} {
		code, out, errw := r.run("check", f)
		if code != exitcode.Data || out != "" || errw == "" {
			t.Errorf("%s: %d %q %q", f, code, out, errw)
		}
		code, out, _ = r.run("--json", "check", f)
		m = doc(t, "check", code, out)
		if m["error"].(map[string]any)["code"] != "data" {
			t.Errorf("%v", m)
		}
	}
}

func TestCatalogThatCannotBeReadIsDataDamage(t *testing.T) {
	r := newRig(t)
	r.d.catalog = func() (*catalog.Catalog, error) { return nil, errors.New("boom") }
	code, out, _ := r.run("--json", "scan")
	m := doc(t, "scan", code, out)
	if code != exitcode.Data || m["error"].(map[string]any)["detail"] != "no-catalog" {
		t.Errorf("%d %v", code, m)
	}
	// --help still works without the catalog
	if code, _, _ := r.run("scan", "--help"); code != 0 {
		t.Error("help needs no catalog")
	}
}

func TestScanWithoutWingetReportsItButStillPrintsWhatItFound(t *testing.T) {
	r := newRig(t)
	r.snapErr = system.ErrNoWinget
	code, out, errw := r.run("scan")
	if code != exitcode.Prereq || !strings.Contains(out, "catalog apps installed") || !strings.Contains(errw, "winget is not available") {
		t.Errorf("%d %q %q", code, out, errw)
	}
	code, out, _ = r.run("scan", "--json")
	m := doc(t, "scan", code, out)
	if code != exitcode.Prereq || m["error"].(map[string]any)["detail"] != "no-winget" || m["installed"] == nil {
		t.Errorf("%v", m)
	}
	// another inventory problem is only a warning
	r.snapErr = errors.New("winget export failed")
	code, out, errw = r.run("scan", "--json")
	m = doc(t, "scan", code, out)
	if code != exitcode.OK || len(m["warnings"].([]any)) != 1 || !strings.Contains(errw, "warning") {
		t.Errorf("%d %v %q", code, m, errw)
	}
	_, _, errw = r.run("scan", "--json", "--quiet")
	if errw != "" {
		t.Errorf("--quiet still printed %q", errw)
	}
}

// ---- apply ------------------------------------------------------------------------------------

func TestApplyJSONReportsEveryStep(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	// after the run the first app shows up as installed, so `to` comes from the rescan
	res, _ := r.cat.Resolve(profile)
	first := install.BuildPlan(r.cat, res, map[string]system.Installed{}).Steps[0]
	r.inv = func(call int) map[string]string {
		if call >= 2 && first.Kind == install.StepApp {
			return map[string]string{strings.ToLower(r.cat.Apps[first.ID].Winget): "9.9"}
		}
		return map[string]string{}
	}
	code, out, errw := r.run("apply", profile, "--json")
	m := doc(t, "apply", code, out)
	if code != 0 || m["rebootRequired"] != false {
		t.Fatalf("%d %v", code, m)
	}
	steps := m["steps"].([]any)
	if len(steps) == 0 {
		t.Fatal("no steps")
	}
	for _, s := range steps {
		s := s.(map[string]any)
		for _, k := range []string{"id", "kind", "name", "result", "reason", "message", "from", "to", "reboot", "admin"} {
			if _, ok := s[k]; !ok {
				t.Errorf("step lacks %s: %v", k, s)
			}
		}
		if s["result"] != "ok" {
			t.Errorf("%v", s)
		}
	}
	if first.Kind == install.StepApp {
		if got := steps[0].(map[string]any)["to"]; got != "9.9" {
			t.Errorf("version after = %v", got)
		}
		if steps[0].(map[string]any)["from"] != "" {
			t.Error("not installed before")
		}
	}
	if c := m["counts"].(map[string]any); c["ok"] != float64(len(steps)) || c["failed"] != float64(0) {
		t.Errorf("%v", c)
	}
	if !strings.Contains(errw, "->") || strings.Contains(out, "->") {
		t.Errorf("progress must go to stderr, not stdout: %q", errw)
	}
	// the human logs disappear with --quiet
	_, out, errw = r.run("apply", profile, "--json", "--quiet")
	doc(t, "apply", 0, out)
	if errw != "" {
		t.Errorf("stderr with --quiet: %q", errw)
	}
	// human mode is as before
	code, out, errw = r.run("apply", profile)
	if code != 0 || !strings.Contains(out, "-> ") || errw != "" {
		t.Errorf("%d %q %q", code, out, errw)
	}
	_, out, _ = r.run("apply", profile, "--quiet")
	if out != "" {
		t.Errorf("--quiet printed %q", out)
	}
}

func TestApplyFailuresAndDependencies(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	r.ex.exit = func(string, []string) int { return 1 }
	code, out, errw := r.run("apply", profile, "--json")
	m := doc(t, "apply", code, out)
	if code != exitcode.Failures || m["error"].(map[string]any)["code"] != "failures" || !strings.Contains(errw, "failed:") {
		t.Fatalf("%d %v", code, m)
	}
	c := m["counts"].(map[string]any)
	if c["failed"].(float64) < 1 {
		t.Errorf("%v", c)
	}
	var failed map[string]any
	for _, s := range m["steps"].([]any) {
		if s.(map[string]any)["result"] == "failed" {
			failed = s.(map[string]any)
			break
		}
	}
	if failed == nil || failed["message"] == "" {
		t.Errorf("%v", failed)
	}
	code, out, errw = r.run("apply", profile, "--quiet")
	if code != exitcode.Failures || out != "" || !strings.Contains(errw, "failed") {
		t.Errorf("quiet: %d %q %q", code, out, errw)
	}
}

func TestApplyRebootIsExit8WhenEverythingElseWorked(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	r.ex.exit = func(string, []string) int { return 3010 }
	code, out, _ := r.run("apply", profile, "--json")
	m := doc(t, "apply", code, out)
	if code != exitcode.Reboot || m["ok"] != true || m["rebootRequired"] != true || m["error"] != nil {
		t.Fatalf("%d %v", code, m)
	}
	if s := m["steps"].([]any)[0].(map[string]any); s["reboot"] != true || s["result"] != "ok" {
		t.Errorf("%v", s)
	}
	// a failure outranks the restart
	n := 0
	r.ex.exit = func(string, []string) int {
		n++
		if n == 1 {
			return 1
		}
		return 3010
	}
	if code, _, _ := r.run("apply", profile); code != exitcode.Failures {
		t.Errorf("%d", code)
	}
}

func TestApplyIsRefusedWhenAdminIsNeededAndNothingMayPrompt(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(true)
	for _, flag := range []string{"--yes", "--non-interactive"} {
		code, out, errw := r.run("apply", profile, flag, "--json")
		m := doc(t, "apply", code, out)
		if code != exitcode.Admin || len(r.ex.calls) != 0 || !strings.Contains(errw, "administrator") {
			t.Fatalf("%s: %d calls=%v", flag, code, r.ex.calls)
		}
		for _, s := range m["steps"].([]any) {
			if s.(map[string]any)["result"] != "skipped" || s.(map[string]any)["reason"] != "admin" {
				t.Errorf("%v", s)
			}
		}
	}
	// elevated: fine
	r.elev = true
	if code, _, _ := r.run("apply", profile, "--yes"); code != 0 || len(r.ex.calls) == 0 {
		t.Errorf("elevated: %d", code)
	}
	// without the flag nothing changes from before
	r.elev, r.ex.calls = false, nil
	if code, _, _ := r.run("apply", profile); code != 0 || len(r.ex.calls) == 0 {
		t.Errorf("no flag: %d", code)
	}
}

func TestApplyNeedsWinget(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	r.snapErr = system.ErrNoWinget
	code, out, _ := r.run("apply", profile, "--json")
	m := doc(t, "apply", code, out)
	if code != exitcode.Prereq || len(r.ex.calls) != 0 || m["error"].(map[string]any)["detail"] != "prerequisite" {
		t.Errorf("%d %v", code, m)
	}
}

func TestApplyBusyWhenAnotherRunHoldsTheLock(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	release, err := sched.Lock{Path: r.lock}.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	code, out, errw := r.run("apply", profile, "--json")
	m := doc(t, "apply", code, out)
	if code != exitcode.Busy || len(r.ex.calls) != 0 || !strings.Contains(errw, "already in progress") || m["error"].(map[string]any)["code"] != "busy" {
		t.Errorf("%d %v", code, m)
	}
	release()
	if code, _, _ := r.run("apply", profile); code != 0 {
		t.Errorf("after release: %d", code)
	}
	if _, err := os.Stat(r.lock); !errors.Is(err, os.ErrNotExist) {
		t.Error("the lock must be released at the end")
	}
}

func TestTimeoutStopsApplyAndIsExit6(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	r.ex.block = true
	start := time.Now()
	code, out, _ := r.run("apply", profile, "--json", "--timeout", "150ms")
	m := doc(t, "apply", code, out)
	if code != exitcode.Timeout || time.Since(start) > 10*time.Second || m["error"].(map[string]any)["detail"] != "timeout" {
		t.Fatalf("%d %v after %s", code, m, time.Since(start))
	}
	for _, s := range m["steps"].([]any) {
		if s.(map[string]any)["result"] == "ok" {
			t.Errorf("%v", s)
		}
	}
	if _, err := os.Stat(r.lock); !errors.Is(err, os.ErrNotExist) {
		t.Error("lock left behind")
	}
}

func TestTimeoutDuringScan(t *testing.T) {
	r := newRig(t)
	r.d.snapshot = func(ctx context.Context) (system.Inventory, error) {
		<-ctx.Done()
		return system.Inventory{Winget: map[string]string{}}, ctx.Err()
	}
	code, out, _ := r.run("scan", "--json", "--timeout=100ms")
	doc(t, "scan", code, out)
	if code != exitcode.Timeout {
		t.Errorf("%d", code)
	}
}

func TestCancellationIsAFailureNotSuccess(t *testing.T) {
	r := newRig(t)
	profile := r.profileWith(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var o, e bytes.Buffer
	if code := run(ctx, []string{"apply", profile}, &o, &e, r.d); code == 0 {
		t.Error("a cancelled run reported success")
	}
}

// ---- updates through the same front door ------------------------------------------------------

func TestUpdatesGetsTheGlobalOptionsFirstOrLast(t *testing.T) {
	var got [][]string
	var gotOpts []cliout.Options
	r := newRig(t)
	r.d.updates = func(args []string, o cliout.Options, out, errw io.Writer) int {
		got, gotOpts = append(got, args), append(gotOpts, o)
		return 0
	}
	r.run("--json", "updates", "--mode", "apply", "--timeout", "5m")
	r.run("updates", "--scheduled", "--quiet")
	if strings.Join(got[0], " ") != "--mode apply" || !gotOpts[0].JSON || gotOpts[0].Timeout != 5*time.Minute ||
		strings.Join(got[1], " ") != "--scheduled" || !gotOpts[1].Quiet {
		t.Errorf("%v %+v", got, gotOpts)
	}
}
