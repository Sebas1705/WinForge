package pcread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/healthhistory"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/privacy"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/tasks"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

const (
	testUser = "sebss"
	testHome = `C:\Users\sebss`
)

var fixedNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// rec records which backends a test touched.
type rec struct {
	mu    sync.Mutex
	calls []string
}

func (r *rec) add(s string) {
	r.mu.Lock()
	r.calls = append(r.calls, s)
	r.mu.Unlock()
}

func (r *rec) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.calls...)
}

func fakeRedact(s string) string {
	s = replaceFold(s, testHome, "~")
	s = replaceFold(s, strings.ReplaceAll(testHome, `\`, "/"), "~")
	return replaceFold(s, testUser, "<user>")
}

func testEnv(admin bool) *mcpkit.Env {
	return &mcpkit.Env{
		Now:      func() time.Time { return fixedNow },
		Elevated: func() bool { return admin },
		Redact:   fakeRedact,
	}
}

func many[T any](n int, mk func(i int) T) []T {
	out := make([]T, n)
	for i := range out {
		out[i] = mk(i)
	}
	return out
}

// fakeDeps is a full set of fakes holding planted personal data, that record what they were asked.
func fakeDeps(r *rec) Deps {
	secretPath := testHome + `\Documents\sebss-taxes.pdf`
	return Deps{
		HealthCollect: func(ctx context.Context, onStep func(string)) (*health.Report, error) {
			r.add("HealthCollect")
			onStep("machine")
			onStep("drivers")
			return &health.Report{
				Machine: health.Machine{Manufacturer: "ACME", Model: "Laptop of sebss"},
				OS:      health.OS{Caption: "Windows 11 Pro"},
				CPU:     health.CPU{Name: "Test CPU"},
				Memory:  health.Memory{TotalGB: 32},
				Problems: []health.Problem{
					{Device: `Unknown device of sebss at ` + testHome, Class: "System", HardwareID: `ACPI\RTK5452\1`, Code: 28},
				},
				Drivers: []health.Driver{{Device: "sebss audio", Class: "MEDIA", Manufacturer: "Realtek", Date: "2019-01-01", Signed: false}},
				Disks:   []health.Disk{{Name: "sebss SSD", Media: "SSD", Health: "Healthy"}},
				Errors:  []string{`could not read ` + secretPath},
			}, nil
		},
		HealthUpdates: func(ctx context.Context) (*health.UpdateScan, error) {
			r.add("HealthUpdates")
			return &health.UpdateScan{CheckedAt: fixedNow, Updates: []health.Update{{Title: "Driver for sebss", Kind: "Driver"}}}, nil
		},
		HealthHistory: func(ctx context.Context) (healthhistory.View, error) {
			r.add("HealthHistory")
			snaps := many(5, func(i int) healthhistory.Snapshot {
				return healthhistory.Snapshot{
					At: fixedNow.Add(time.Duration(i-5) * 48 * time.Hour), Score: 15 + i, Total: 20,
					Areas:   map[string]int{"storage": 1},
					Volumes: []healthhistory.Volume{{Letter: "C:", FreeBytes: 100 << 30, TotalBytes: 500 << 30}},
					Disks:   []healthhistory.Disk{{Name: "sebss SSD", Media: "SSD", Health: "Healthy"}},
				}
			})
			return healthhistory.View{
				Snapshots:  snaps,
				Statements: []healthhistory.Statement{{Code: "free-dropped", Params: map[string]string{"drive": "C: " + testHome}}},
				Enough:     true,
			}, nil
		},
		Battery: func(ctx context.Context) (battery.Report, error) {
			r.add("Battery")
			c := 120
			return battery.Report{
				Present: true, HealthPct: 71.5, Cycles: &c, Verdict: battery.VerdictWorn, DesignMWh: 50000, FullMWh: 35750,
				Batteries:  []battery.Battery{{Chemistry: "LION", DesignMWh: 50000, FullMWh: 35750, HealthPct: 71.5, Cycles: &c, Verdict: "worn"}},
				History:    []battery.Point{{Date: "2026-11-01", FullMWh: 36000, HealthPct: 72}},
				GoodMinPct: 80, ReplaceBelowPct: 60,
			}, nil
		},
		Volumes: func() []storage.Volume {
			r.add("Volumes")
			return []storage.Volume{{Letter: "C:", Label: "sebss disk", TotalBytes: 500 << 30, FreeBytes: 100 << 30}}
		},
		StorageScan: func(ctx context.Context, admin bool, progress storage.ProgressFunc) ([]storage.Category, error) {
			r.add("StorageScan")
			progress(storage.Progress{Task: "scan", Files: 10, Current: secretPath})
			return []storage.Category{
				{ID: "temp", Risk: "safe", NeedsAdmin: false, Available: true, Bytes: 1234, Files: 5, Skipped: 1},
				{ID: "windows-old", Risk: "careful", NeedsAdmin: true, Available: false},
			}, nil
		},
		BigFiles: func(ctx context.Context, root string, limit int, progress storage.ProgressFunc) ([]storage.BigFile, error) {
			r.add("BigFiles")
			progress(storage.Progress{Task: "big", Files: 3, Current: secretPath})
			return many(min(limit, 400), func(i int) storage.BigFile {
				return storage.BigFile{Path: fmt.Sprintf(`%s\Videos\sebss-%d.mkv`, testHome, i), Name: fmt.Sprintf("sebss-%d.mkv", i), SizeBytes: int64(1000 - i), Modified: fixedNow}
			}), nil
		},
		FolderSizes: func(ctx context.Context, root string, progress storage.ProgressFunc) ([]storage.FolderSize, error) {
			r.add("FolderSizes")
			return many(60, func(i int) storage.FolderSize {
				return storage.FolderSize{Path: fmt.Sprintf(`%s\sebss-dir%d`, testHome, i), Name: fmt.Sprintf("sebss-dir%d", i), SizeBytes: int64(500 - i), Files: i}
			}), nil
		},
		ProjectJunk: func(ctx context.Context, roots []string, minAge time.Duration, now time.Time, progress storage.ProgressFunc) ([]storage.ProjectJunk, error) {
			r.add("ProjectJunk")
			return []storage.ProjectJunk{{Path: testHome + `\code\sebss-app\node_modules`, Project: "sebss-app", ProjectPath: testHome + `\code\sebss-app`, Kind: "node", SizeBytes: 999, Files: 3, LastTouched: fixedNow.Add(-100 * 24 * time.Hour)}}, nil
		},
		VirtualDisks: func() []storage.VirtualDisk {
			r.add("VirtualDisks")
			return []storage.VirtualDisk{{Path: testHome + `\AppData\Local\Packages\Ubuntu\ext4.vhdx`, Name: "Ubuntu-sebss", Kind: "wsl", SizeBytes: 1 << 30, Modified: fixedNow}}
		},
		CheckFolder: func(p string) error {
			r.add("CheckFolder")
			if strings.Contains(p, "missing") {
				return errors.New("no")
			}
			return nil
		},
		StartupList: func() ([]startup.Item, error) {
			r.add("StartupList")
			return many(500, func(i int) startup.Item {
				return startup.Item{ID: fmt.Sprintf("hkcu|app%d", i), Name: fmt.Sprintf("app%d", i), Source: "hkcu-run", Command: testHome + `\AppData\Local\app.exe --token sebss ` + strings.Repeat("x", 600), Target: testHome + `\AppData\Local\app.exe`, Publisher: "Pub", Enabled: true}
			}), nil
		},
		ServicesList: func() ([]services.Item, error) {
			r.add("ServicesList")
			return []services.Item{{ID: "svc", Name: "svc", DisplayName: "Service of sebss", State: "Running", StartMode: "auto", Binary: testHome + `\svc.exe`, Dependents: []string{"dep sebss"}}}, nil
		},
		TasksList: func() ([]tasks.Item, error) {
			r.add("TasksList")
			code := int32(0)
			return []tasks.Item{{ID: `\Vendor\Updater`, Name: "Updater", Folder: `\Vendor\`, Triggers: []string{"At logon of sebss"}, Action: testHome + `\up.exe`, State: "Ready", Enabled: true, LastResult: &code}}, nil
		},
		EnvScan: func() (envdoctor.Report, error) {
			r.add("EnvScan")
			return envdoctor.Report{
				Findings:    []envdoctor.Finding{{ID: "path-missing|" + testHome, Kind: "path-missing", Severity: "warn", Scope: envdoctor.User, Subject: testHome + `\bin`, Paths: []string{testHome + `\bin`}, Fixable: true, FixKind: "remove-path"}},
				UserPath:    []string{testHome + `\bin`, `C:\Tools`},
				MachinePath: []string{`C:\Windows`},
				Tools:       []envdoctor.Tool{{Name: "node", Found: []string{testHome + `\node\node.exe`}}},
				Vars:        []envdoctor.Var{{Name: "JAVA_HOME", Scope: envdoctor.User, Value: testHome + `\jdk`}},
			}, nil
		},
		FeaturesList: func(elevated bool) []winfeatures.Feature {
			r.add(fmt.Sprintf("FeaturesList(admin=%v)", elevated))
			return []winfeatures.Feature{{ID: "wsl", State: "on", Enable: "feature-wsl-on", Disable: "feature-wsl-off"}}
		},
		WSLInfo: func() winfeatures.WSLInfo {
			r.add("WSLInfo")
			return winfeatures.WSLInfo{Installed: true, DefaultVersion: 2, Distros: []winfeatures.Distro{{Name: "Ubuntu-sebss", State: "Running", Version: 2, IsDefault: true}}}
		},
		PrivacyList: func() ([]privacy.TweakView, error) {
			r.add("PrivacyList")
			return []privacy.TweakView{{ID: "ads", Area: "ads", NameKey: "privacy.ads", WhyKey: "privacy.ads.why", State: "default",
				Changes: []privacy.ChangeView{{Path: `HKEY_CURRENT_USER\Software\sebss\Ads`, Name: "Enabled", Existed: true, OldType: "REG_DWORD", Old: "1", Type: "REG_DWORD", Private: 0}}}}, nil
		},
	}
}

// readOnly lists every backend a read tool may touch; anything else a fake records is a failure.
var readBackends = map[string]bool{
	"HealthCollect": true, "HealthUpdates": true, "HealthHistory": true, "Battery": true, "Volumes": true,
	"StorageScan": true, "BigFiles": true, "FolderSizes": true, "ProjectJunk": true, "VirtualDisks": true, "CheckFolder": true,
	"StartupList": true, "ServicesList": true, "TasksList": true, "EnvScan": true, "WSLInfo": true, "PrivacyList": true,
	"FeaturesList(admin=false)": true, "FeaturesList(admin=true)": true,
}

type harness struct {
	t     *testing.T
	rec   *rec
	tools map[string]mcpkit.Tool
	env   *mcpkit.Env
}

func newHarness(t *testing.T, mod func(d *Deps)) *harness {
	t.Helper()
	r := &rec{}
	d := fakeDeps(r)
	if mod != nil {
		mod(&d)
	}
	env := testEnv(false)
	ts := ToolsWith(env, d)
	m := map[string]mcpkit.Tool{}
	for _, tl := range ts {
		m[tl.Name] = tl
	}
	return &harness{t: t, rec: r, tools: m, env: env}
}

// call runs a tool and checks that nothing personal leaked and that only read backends were used.
func (h *harness) call(name, args string) *mcpkit.Result {
	h.t.Helper()
	return h.callCtx(context.Background(), name, args, nil)
}

func (h *harness) callCtx(ctx context.Context, name, args string, progress func(done, total float64, msg string)) *mcpkit.Result {
	h.t.Helper()
	tl, ok := h.tools[name]
	if !ok {
		h.t.Fatalf("no tool %s", name)
	}
	res, err := tl.Handler(ctx, &mcpkit.Call{Args: json.RawMessage(args), Env: h.env, Progress: progress})
	if err != nil {
		h.t.Fatalf("%s returned a Go error %v; errors must be tool results", name, err)
	}
	if res == nil {
		h.t.Fatalf("%s returned nil", name)
	}
	noLeak(h.t, name, res)
	for _, c := range h.rec.got() {
		if !readBackends[c] {
			h.t.Errorf("%s touched a non-read backend %q", name, c)
		}
	}
	return res
}

func noLeak(t *testing.T, name string, res *mcpkit.Result) {
	t.Helper()
	b, _ := json.Marshal(res.Structured)
	all := strings.ToLower(res.Text + string(b))
	for _, bad := range []string{testUser, strings.ToLower(testHome), strings.ToLower(strings.ReplaceAll(testHome, `\`, `\\`)), "users/sebss"} {
		if strings.Contains(all, bad) {
			t.Errorf("%s leaked %q in:\n%s", name, bad, res.Text)
		}
	}
}

func structured(t *testing.T, res *mcpkit.Result) map[string]any {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Text)
	}
	b, err := json.Marshal(res.Structured)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(res.Text)) {
		t.Errorf("Text is not JSON: %s", res.Text)
	}
	return m
}

func items(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	l, ok := m[key].([]any)
	if !ok {
		t.Fatalf("%s is not a list in %v", key, m)
	}
	return l
}

var allTools = []string{
	"winforge_health_scan", "winforge_health_history", "winforge_battery",
	"winforge_storage_volumes", "winforge_storage_scan", "winforge_storage_biggest", "winforge_storage_projects", "winforge_virtual_disks",
	"winforge_startup_list", "winforge_services_list", "winforge_tasks_list",
	"winforge_env_scan", "winforge_features_list", "winforge_wsl_info", "winforge_privacy_list",
}

func TestToolList(t *testing.T) {
	h := newHarness(t, nil)
	if len(h.tools) != len(allTools) {
		t.Errorf("%d tools, want %d", len(h.tools), len(allTools))
	}
	for _, n := range allTools {
		tl, ok := h.tools[n]
		if !ok {
			t.Errorf("missing tool %s", n)
			continue
		}
		if tl.Class != mcpkit.Read {
			t.Errorf("%s is not a Read tool", n)
		}
		if tl.Title == "" || tl.Handler == nil {
			t.Errorf("%s incomplete", n)
		}
		if !strings.Contains(tl.Description, "NEVER") {
			t.Errorf("%s description does not say what it never does", n)
		}
		var s map[string]any
		if err := json.Unmarshal(tl.InputSchema, &s); err != nil || s["additionalProperties"] != false || s["type"] != "object" {
			t.Errorf("%s schema is not strict: %s", n, tl.InputSchema)
		}
	}
	// Tools that return paths or names from this PC must say who sees them.
	for _, n := range []string{"winforge_storage_biggest", "winforge_storage_projects", "winforge_virtual_disks", "winforge_startup_list", "winforge_env_scan", "winforge_privacy_list"} {
		if !strings.Contains(h.tools[n].Description, "seen by the model") && !strings.Contains(h.tools[n].Description, "model sees") {
			t.Errorf("%s description does not state that the model sees the result", n)
		}
	}
	if got := len(Tools(testEnv(false))); got != len(allTools) {
		t.Errorf("real Tools has %d tools", got)
	}
}

// Every tool, with its default arguments, must work on the fakes, never leak the planted data and only read.
func TestEveryToolDefaults(t *testing.T) {
	args := map[string]string{
		"winforge_storage_biggest":  `{"root":"C:\\Data","kind":"files"}`,
		"winforge_storage_projects": `{"roots":["C:\\Data"]}`,
	}
	for _, n := range allTools {
		t.Run(n, func(t *testing.T) {
			h := newHarness(t, nil)
			a := args[n]
			if a == "" {
				a = "{}"
			}
			res := h.call(n, a)
			if res.IsError {
				t.Fatalf("error: %s", res.Text)
			}
			if res.Structured == nil || !json.Valid([]byte(res.Text)) {
				t.Errorf("no structured result")
			}
			if len(h.rec.got()) == 0 {
				t.Errorf("did not call its backend")
			}
		})
	}
}

func TestArgumentValidation(t *testing.T) {
	bad := map[string][]string{
		"winforge_health_scan":      {`{"nope":1}`, `{"withWindowsUpdate":"yes"}`, `{"limit":"5"}`, `{"limit":0}`, `{"limit":201}`, `{"limit":-3}`, `{"limit":1.5}`, `{} {}`, `[]`},
		"winforge_health_history":   {`{"x":1}`, `{"limit":100000}`},
		"winforge_battery":          {`{"x":1}`},
		"winforge_storage_volumes":  {`{"x":1}`},
		"winforge_storage_scan":     {`{"all":true}`},
		"winforge_storage_biggest":  {`{}`, `{"root":"C:\\Data"}`, `{"kind":"files"}`, `{"root":5,"kind":"files"}`, `{"root":"C:\\Data","kind":"both"}`, `{"root":"C:\\Data","kind":"files","limit":99999}`, `{"root":"C:\\Data","kind":"files","extra":1}`, `{"root":"C:\\Data","kind":"files","allowDriveRoot":"yes"}`},
		"winforge_storage_projects": {`{}`, `{"roots":[]}`, `{"roots":"C:\\Data"}`, `{"roots":["C:\\Data"],"minDays":0}`, `{"roots":["C:\\Data"],"minDays":99999}`, `{"roots":["C:\\Data"],"limit":500}`, `{"roots":["C:\\1","C:\\2","C:\\3","C:\\4","C:\\5","C:\\6","C:\\7","C:\\8","C:\\9"]}`, `{"roots":[1]}`},
		"winforge_virtual_disks":    {`{"limit":0}`},
		"winforge_startup_list":     {`{"limit":201}`, `{"q":1}`},
		"winforge_services_list":    {`{"limit":"x"}`},
		"winforge_tasks_list":       {`{"limit":null,"z":1}`},
		"winforge_env_scan":         {`{"includePaths":1}`},
		"winforge_features_list":    {`{"x":1}`},
		"winforge_wsl_info":         {`{"limit":0}`},
		"winforge_privacy_list":     {`{"limit":300}`},
	}
	for n, list := range bad {
		for _, a := range list {
			t.Run(n+a, func(t *testing.T) {
				h := newHarness(t, nil)
				res := h.call(n, a)
				if !res.IsError {
					t.Fatalf("accepted %s", a)
				}
				if got := h.rec.got(); len(got) != 0 {
					t.Errorf("backend ran before the arguments were valid: %v", got)
				}
			})
		}
	}
}

func TestFolderRules(t *testing.T) {
	bad := []string{
		`Documents`, `.\Documents`, `..\x`, `C:Documents`, `C:`, `~/x`,
		`\\server\share\x`, `//server/share`, `\\?\C:\Users`, `\\.\PhysicalDrive0`, `\\.\C:`,
		`C:\a\..\b`, `C:\..\..\Windows`, `C:\Users\me\..`,
		`C:\CON`, `C:\x\NUL`, `C:\x\com1.txt`, `C:\LPT9`, `C:\Data\file:stream`, `C:\Da*ta`, `C:\Da?ta`, `C:\a|b`, `C:\a<b`, "C:\\a\x00b", "C:\\a\nb", ``, `   `,
	}
	for _, p := range bad {
		if got, err := checkFolder(p, true); err == nil {
			t.Errorf("accepted %q -> %q", p, got)
		}
	}
	for _, p := range []string{`C:\Users\me\Downloads`, `D:/Data/stuff`, `c:\x`, `C:\Program Files (x86)\App`, `C:\console\x`} {
		if _, err := checkFolder(p, false); err != nil {
			t.Errorf("refused %q: %v", p, err)
		}
	}
	for _, p := range []string{`C:\`, `C:/`, `d:\`} {
		if _, err := checkFolder(p, false); err == nil {
			t.Errorf("drive root %q accepted without allowDriveRoot", p)
		}
		if _, err := checkFolder(p, true); err != nil {
			t.Errorf("drive root %q refused with allowDriveRoot: %v", p, err)
		}
	}
}

func TestBiggestPathsThroughTool(t *testing.T) {
	h := newHarness(t, nil)
	for _, root := range []string{`Documents`, `\\\\srv\\share`, `\\\\?\\C:\\x`, `C:\\a\\..\\b`, `C:\\`, `C:\\CON`, `C:\\missing\\dir`} {
		res := h.call("winforge_storage_biggest", `{"root":"`+root+`","kind":"files"}`)
		if !res.IsError {
			t.Errorf("root %s accepted", root)
		}
	}
	for _, c := range h.rec.got() {
		if c == "BigFiles" || c == "FolderSizes" {
			t.Fatalf("scan ran for a refused path (%s)", c)
		}
	}
	// A whole drive is fine with the flag.
	res := h.call("winforge_storage_biggest", `{"root":"C:\\","kind":"files","allowDriveRoot":true,"limit":3}`)
	if res.IsError {
		t.Fatalf("drive root with flag: %s", res.Text)
	}
}

func TestBiggestCapsAndTruncation(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"files"}`))
	if n := len(items(t, m, "items")); n != 50 || m["truncated"] != true {
		t.Errorf("default: %d items truncated=%v", n, m["truncated"])
	}
	m = structured(t, h.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"files","limit":200}`))
	if n := len(items(t, m, "items")); n != 200 || m["truncated"] != true {
		t.Errorf("limit 200: %d items truncated=%v", n, m["truncated"])
	}
	// The backend returns fewer than asked: not truncated.
	h2 := newHarness(t, func(d *Deps) {
		d.BigFiles = func(ctx context.Context, root string, limit int, p storage.ProgressFunc) ([]storage.BigFile, error) {
			return []storage.BigFile{{Path: `C:\Data\a.bin`, Name: "a.bin", SizeBytes: 5}}, nil
		}
	})
	m = structured(t, h2.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"files"}`))
	if n := len(items(t, m, "items")); n != 1 || m["truncated"] != false {
		t.Errorf("short list: %d truncated=%v", n, m["truncated"])
	}
	// The backend is asked for one more than the limit, to know whether it was cut.
	var asked int
	h3 := newHarness(t, func(d *Deps) {
		d.BigFiles = func(ctx context.Context, root string, limit int, p storage.ProgressFunc) ([]storage.BigFile, error) {
			asked = limit
			return nil, nil
		}
	})
	h3.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"files","limit":7}`)
	if asked != 8 {
		t.Errorf("backend asked for %d, want 8", asked)
	}
	// Folders: the backend caps at 60, so a full 60 means "maybe more".
	m = structured(t, h.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"folders","limit":200}`))
	if n := len(items(t, m, "items")); n != 60 || m["truncated"] != true {
		t.Errorf("folders: %d truncated=%v", n, m["truncated"])
	}
	m = structured(t, h.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"folders","limit":5}`))
	if n := len(items(t, m, "items")); n != 5 || m["truncated"] != true {
		t.Errorf("folders limit 5: %d truncated=%v", n, m["truncated"])
	}
	if m["root"] != `C:\Data` {
		t.Errorf("root = %v", m["root"])
	}
}

func TestListCaps(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_startup_list", `{}`))
	if n := len(items(t, m, "items")); n != 50 || m["truncated"] != true {
		t.Errorf("default %d truncated=%v", n, m["truncated"])
	}
	m = structured(t, h.call("winforge_startup_list", `{"limit":200}`))
	if n := len(items(t, m, "items")); n != 200 || m["truncated"] != true {
		t.Errorf("200: %d truncated=%v", n, m["truncated"])
	}
	m = structured(t, h.call("winforge_startup_list", `{"limit":1}`))
	if n := len(items(t, m, "items")); n != 1 {
		t.Errorf("1: %d", n)
	}
	first := items(t, m, "items")[0].(map[string]any)
	if cmd := first["command"].(string); len(cmd) > maxText+10 {
		t.Errorf("command not shortened: %d bytes", len(cmd))
	}
	// Short lists are not truncated.
	m = structured(t, h.call("winforge_services_list", `{}`))
	if n := len(items(t, m, "items")); n != 1 || m["truncated"] != false {
		t.Errorf("services: %d truncated=%v", n, m["truncated"])
	}
}

func TestHealthScan(t *testing.T) {
	h := newHarness(t, nil)
	var msgs []string
	res := h.callCtx(context.Background(), "winforge_health_scan", `{}`, func(d, tot float64, msg string) { msgs = append(msgs, msg) })
	m := structured(t, res)
	score := m["score"].(map[string]any)
	if score["total"].(float64) < 5 {
		t.Errorf("score %v", score)
	}
	fs := items(t, m, "findings")
	if len(fs) == 0 {
		t.Fatal("no findings")
	}
	f0 := fs[0].(map[string]any)
	for _, k := range []string{"key", "group", "severity"} {
		if _, ok := f0[k]; !ok {
			t.Errorf("finding lacks %s", k)
		}
	}
	mach := m["machine"].(map[string]any)
	if mach["manufacturer"] != "ACME" || mach["ramGB"].(float64) != 32 || mach["os"] != "Windows 11 Pro" || mach["cpu"] != "Test CPU" {
		t.Errorf("machine %v", mach)
	}
	if mach["model"] != "Laptop of <user>" {
		t.Errorf("model not redacted: %v", mach["model"])
	}
	if m["windowsUpdateSearched"] != false {
		t.Error("windows update searched by default")
	}
	for _, c := range h.rec.got() {
		if c == "HealthUpdates" {
			t.Error("searched Windows Update without being asked")
		}
	}
	if len(msgs) < 3 {
		t.Errorf("progress messages: %v", msgs)
	}
	// The planted device name must show up redacted, not vanish: the test is not vacuous.
	if !strings.Contains(res.Text, "003cuser") {
		t.Errorf("planted data did not reach the output (test would pass vacuously):\n%s", res.Text)
	}
	// With Windows Update.
	h2 := newHarness(t, nil)
	m = structured(t, h2.call("winforge_health_scan", `{"withWindowsUpdate":true,"limit":3}`))
	if m["windowsUpdateSearched"] != true || len(items(t, m, "findings")) != 3 || m["truncated"] != true {
		t.Errorf("with updates: %v", m["windowsUpdateSearched"])
	}
	called := strings.Join(h2.rec.got(), ",")
	if !strings.Contains(called, "HealthUpdates") {
		t.Errorf("updates not searched: %s", called)
	}
}

func TestHealthHistory(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_health_history", `{"limit":2}`))
	sn := m["snapshots"].(map[string]any)
	if sn["count"].(float64) != 5 {
		t.Errorf("count %v", sn["count"])
	}
	tl := items(t, m, "timeline")
	if len(tl) != 2 || m["truncated"] != true {
		t.Errorf("timeline %d truncated=%v", len(tl), m["truncated"])
	}
	// The newest points are kept.
	if tl[1].(map[string]any)["score"].(float64) != 19 {
		t.Errorf("timeline = %v", tl)
	}
	st := items(t, m, "statements")
	if len(st) != 1 || st[0].(map[string]any)["code"] != "free-dropped" {
		t.Errorf("statements %v", st)
	}
	if !strings.Contains(fmt.Sprint(st), "<user>") && !strings.Contains(fmt.Sprint(st), "~") {
		t.Errorf("statement params not shown redacted: %v", st)
	}
	// An empty history.
	h2 := newHarness(t, func(d *Deps) {
		d.HealthHistory = func(context.Context) (healthhistory.View, error) {
			return healthhistory.BuildView(healthhistory.File{}), nil
		}
	})
	m = structured(t, h2.call("winforge_health_history", `{}`))
	if m["snapshots"].(map[string]any)["count"].(float64) != 0 || m["latest"] != nil {
		t.Errorf("empty history: %v", m)
	}
}

func TestBattery(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_battery", `{}`))
	if m["present"] != true || m["healthPct"].(float64) != 71.5 || m["verdict"] != "worn" || m["cycles"].(float64) != 120 {
		t.Errorf("battery %v", m)
	}
	if th := m["thresholds"].(map[string]any); th["goodMinPct"].(float64) != 80 {
		t.Errorf("thresholds %v", th)
	}
	// No battery.
	h2 := newHarness(t, func(d *Deps) {
		d.Battery = func(context.Context) (battery.Report, error) { return battery.Report{Verdict: "unknown"}, nil }
	})
	m = structured(t, h2.call("winforge_battery", `{}`))
	if m["present"] != false || len(items(t, m, "batteries")) != 0 || m["cycles"] != nil {
		t.Errorf("no battery: %v", m)
	}
}

func TestStorageScanAndVolumes(t *testing.T) {
	h := newHarness(t, nil)
	var msgs []string
	res := h.callCtx(context.Background(), "winforge_storage_scan", `{}`, func(d, tot float64, msg string) { msgs = append(msgs, msg) })
	m := structured(t, res)
	cats := items(t, m, "categories")
	if len(cats) != 2 {
		t.Fatalf("categories %v", cats)
	}
	c := cats[0].(map[string]any)
	if c["id"] != "temp" || c["risk"] != "safe" || c["bytes"].(float64) != 1234 || c["files"].(float64) != 5 || c["skipped"].(float64) != 1 || c["available"] != true || c["needsAdmin"] != false {
		t.Errorf("category %v", c)
	}
	if m["totalBytes"].(float64) != 1234 {
		t.Errorf("total %v", m["totalBytes"])
	}
	joined := strings.Join(msgs, "|")
	if !strings.Contains(joined, "scanning") || strings.Contains(strings.ToLower(joined), testUser) {
		t.Errorf("progress %q", joined)
	}
	m = structured(t, h.call("winforge_storage_volumes", `{}`))
	v := items(t, m, "volumes")[0].(map[string]any)
	if v["letter"] != "C:" || v["label"] != "<user> disk" || v["freeBytes"].(float64) != float64(int64(100)<<30) {
		t.Errorf("volume %v", v)
	}
	// Admin flag reaches the scan.
	var gotAdmin bool
	h2 := newHarness(t, func(d *Deps) {
		d.StorageScan = func(ctx context.Context, admin bool, p storage.ProgressFunc) ([]storage.Category, error) {
			gotAdmin = admin
			return nil, nil
		}
	})
	h2.env.Elevated = func() bool { return true }
	m = structured(t, h2.call("winforge_storage_scan", `{}`))
	if !gotAdmin || m["ranAsAdmin"] != true || len(items(t, m, "categories")) != 0 {
		t.Errorf("admin %v %v", gotAdmin, m)
	}
}

func TestProjectsAndVirtualDisks(t *testing.T) {
	var gotRoots []string
	var gotAge time.Duration
	h := newHarness(t, func(d *Deps) {
		base := d.ProjectJunk
		d.ProjectJunk = func(ctx context.Context, roots []string, age time.Duration, now time.Time, p storage.ProgressFunc) ([]storage.ProjectJunk, error) {
			gotRoots, gotAge = roots, age
			if !now.Equal(fixedNow) {
				t.Errorf("now = %v", now)
			}
			return base(ctx, roots, age, now, p)
		}
	})
	m := structured(t, h.call("winforge_storage_projects", `{"roots":["C:\\code","D:/more"],"minDays":30}`))
	if len(gotRoots) != 2 || gotAge != 30*24*time.Hour {
		t.Errorf("roots %v age %v", gotRoots, gotAge)
	}
	it := items(t, m, "items")[0].(map[string]any)
	if it["project"] != "<user>-app" || it["kind"] != "node" || it["sizeBytes"].(float64) != 999 {
		t.Errorf("item %v", it)
	}
	m = structured(t, h.call("winforge_storage_projects", `{"roots":["C:\\code"]}`))
	if m["minDays"].(float64) != 90 || gotAge != 90*24*time.Hour {
		t.Errorf("default days %v", m["minDays"])
	}
	// One bad root refuses the whole call, and the error does not echo the user name.
	res := h.call("winforge_storage_projects", `{"roots":["C:\\code","\\\\srv\\sebss"]}`)
	if !res.IsError {
		t.Error("UNC root accepted")
	}
	m = structured(t, h.call("winforge_virtual_disks", `{}`))
	d := items(t, m, "disks")[0].(map[string]any)
	if d["kind"] != "wsl" || d["name"] != "Ubuntu-<user>" || !strings.HasPrefix(d["path"].(string), "~") {
		t.Errorf("disk %v", d)
	}
}

func TestStartupServicesTasks(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_startup_list", `{"limit":1}`))
	it := items(t, m, "items")[0].(map[string]any)
	if it["name"] != "app0" || it["enabled"] != true || it["publisher"] != "Pub" || it["source"] != "hkcu-run" {
		t.Errorf("startup %v", it)
	}
	m = structured(t, h.call("winforge_services_list", `{}`))
	s := items(t, m, "items")[0].(map[string]any)
	if s["displayName"] != "Service of <user>" || s["state"] != "Running" || s["startMode"] != "auto" {
		t.Errorf("service %v", s)
	}
	if _, has := s["changed"]; has {
		t.Error("service output carries WinForge's own state")
	}
	m = structured(t, h.call("winforge_tasks_list", `{}`))
	k := items(t, m, "items")[0].(map[string]any)
	if k["name"] != "Updater" || k["lastResult"].(float64) != 0 || k["enabled"] != true {
		t.Errorf("task %v", k)
	}
}

func TestEnvScan(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_env_scan", `{}`))
	f := items(t, m, "findings")[0].(map[string]any)
	if f["kind"] != "path-missing" || f["fixable"] != true || f["scope"] != "user" && f["scope"] != string(envdoctor.User) {
		t.Errorf("finding %v", f)
	}
	for _, k := range []string{"userPath", "machinePath", "tools", "homeVars"} {
		if _, has := m[k]; has {
			t.Errorf("%s returned without includePaths", k)
		}
	}
	if m["counts"].(map[string]any)["userPathEntries"].(float64) != 2 {
		t.Errorf("counts %v", m["counts"])
	}
	m = structured(t, h.call("winforge_env_scan", `{"includePaths":true}`))
	up := items(t, m, "userPath")
	if len(up) != 2 || !strings.HasPrefix(up[0].(string), "~") {
		t.Errorf("userPath %v", up)
	}
	if v := items(t, m, "homeVars")[0].(map[string]any); v["name"] != "JAVA_HOME" || !strings.HasPrefix(v["value"].(string), "~") {
		t.Errorf("var %v", v)
	}
}

func TestFeaturesWSLPrivacy(t *testing.T) {
	h := newHarness(t, nil)
	m := structured(t, h.call("winforge_features_list", `{}`))
	f := items(t, m, "features")[0].(map[string]any)
	if f["id"] != "wsl" || f["state"] != "on" {
		t.Errorf("feature %v", f)
	}
	if _, has := f["enable"]; has {
		t.Error("recipe ids leaked")
	}
	if got := strings.Join(h.rec.got(), ","); !strings.Contains(got, "admin=false") {
		t.Errorf("elevation not passed: %s", got)
	}
	m = structured(t, h.call("winforge_wsl_info", `{}`))
	d := items(t, m, "distros")[0].(map[string]any)
	if d["name"] != "Ubuntu-<user>" || d["state"] != "Running" || d["isDefault"] != true || m["defaultVersion"].(float64) != 2 {
		t.Errorf("wsl %v", m)
	}
	// WSL missing: empty list, not null.
	h2 := newHarness(t, func(d *Deps) {
		d.WSLInfo = func() winfeatures.WSLInfo { return winfeatures.WSLInfo{DefaultVersion: 2} }
	})
	m = structured(t, h2.call("winforge_wsl_info", `{}`))
	if m["distros"] == nil || len(items(t, m, "distros")) != 0 || m["installed"] != false {
		t.Errorf("no wsl: %v", m)
	}
	m = structured(t, h.call("winforge_privacy_list", `{}`))
	p := items(t, m, "settings")[0].(map[string]any)
	if p["id"] != "ads" || p["state"] != "default" {
		t.Errorf("privacy %v", p)
	}
	c := p["changes"].([]any)[0].(map[string]any)
	if c["name"] != "Enabled" || c["old"] != "1" || c["type"] != "REG_DWORD" || c["private"].(float64) != 0 || !strings.Contains(c["path"].(string), "<user>") {
		t.Errorf("change %v", c)
	}
}

func TestErrorMapping(t *testing.T) {
	boom := errors.New(`boom reading ` + testHome + `\secret for sebss`)
	h := newHarness(t, func(d *Deps) {
		d.HealthCollect = func(context.Context, func(string)) (*health.Report, error) { return nil, boom }
		d.HealthUpdates = func(context.Context) (*health.UpdateScan, error) { return nil, boom }
		d.HealthHistory = func(context.Context) (healthhistory.View, error) { return healthhistory.View{}, boom }
		d.Battery = func(context.Context) (battery.Report, error) { return battery.Report{}, boom }
		d.StorageScan = func(context.Context, bool, storage.ProgressFunc) ([]storage.Category, error) { return nil, boom }
		d.BigFiles = func(context.Context, string, int, storage.ProgressFunc) ([]storage.BigFile, error) { return nil, boom }
		d.FolderSizes = func(context.Context, string, storage.ProgressFunc) ([]storage.FolderSize, error) { return nil, boom }
		d.ProjectJunk = func(context.Context, []string, time.Duration, time.Time, storage.ProgressFunc) ([]storage.ProjectJunk, error) {
			return nil, boom
		}
		d.StartupList = func() ([]startup.Item, error) { return nil, boom }
		d.ServicesList = func() ([]services.Item, error) { return nil, boom }
		d.TasksList = func() ([]tasks.Item, error) { return nil, boom }
		d.EnvScan = func() (envdoctor.Report, error) { return envdoctor.Report{}, boom }
		d.PrivacyList = func() ([]privacy.TweakView, error) { return nil, boom }
	})
	cases := map[string]string{
		"winforge_health_scan": `{}`, "winforge_health_history": `{}`, "winforge_battery": `{}`, "winforge_storage_scan": `{}`,
		"winforge_storage_biggest":  `{"root":"C:\\Data","kind":"files"}`,
		"winforge_storage_projects": `{"roots":["C:\\Data"]}`,
		"winforge_startup_list":     `{}`, "winforge_services_list": `{}`, "winforge_tasks_list": `{}`, "winforge_env_scan": `{}`, "winforge_privacy_list": `{}`,
	}
	for n, a := range cases {
		res := h.call(n, a)
		if !res.IsError || !strings.Contains(res.Text, "boom") || !strings.Contains(res.Text, n) {
			t.Errorf("%s: %+v", n, res)
		}
	}
	res := h.call("winforge_storage_biggest", `{"root":"C:\\Data","kind":"folders"}`)
	if !res.IsError {
		t.Error("folders error not mapped")
	}
}

func TestPanicBecomesToolError(t *testing.T) {
	h := newHarness(t, func(d *Deps) {
		d.Battery = func(context.Context) (battery.Report, error) { panic("kaboom sebss") }
		d.StartupList = func() ([]startup.Item, error) { panic("kaboom") }
		d.StorageScan = func(context.Context, bool, storage.ProgressFunc) ([]storage.Category, error) { panic("x") }
		d.HealthCollect = func(context.Context, func(string)) (*health.Report, error) { return nil, nil }
	})
	for _, n := range []string{"winforge_battery", "winforge_startup_list", "winforge_storage_scan", "winforge_health_scan"} {
		res := h.call(n, `{}`)
		if !res.IsError {
			t.Errorf("%s: %+v", n, res)
		}
	}
	// A nil Call does not crash either.
	res, err := h.tools["winforge_battery"].Handler(context.Background(), nil)
	if err != nil || res == nil {
		t.Errorf("nil call: %v %v", res, err)
	}
}

func TestCancellation(t *testing.T) {
	block := func(ctx context.Context) { <-ctx.Done() }
	release := make(chan struct{})
	defer close(release)
	h := newHarness(t, func(d *Deps) {
		d.HealthCollect = func(ctx context.Context, f func(string)) (*health.Report, error) {
			block(ctx)
			return nil, ctx.Err()
		}
		d.StorageScan = func(ctx context.Context, a bool, p storage.ProgressFunc) ([]storage.Category, error) {
			block(ctx)
			return nil, ctx.Err()
		}
		d.BigFiles = func(ctx context.Context, r string, l int, p storage.ProgressFunc) ([]storage.BigFile, error) {
			block(ctx)
			return nil, ctx.Err()
		}
		d.ProjectJunk = func(ctx context.Context, r []string, a time.Duration, n time.Time, p storage.ProgressFunc) ([]storage.ProjectJunk, error) {
			block(ctx)
			return nil, ctx.Err()
		}
		// Backends that take no context: the tool must stop waiting by itself.
		d.StartupList = func() ([]startup.Item, error) { <-release; return nil, nil }
		d.ServicesList = func() ([]services.Item, error) { <-release; return nil, nil }
		d.TasksList = func() ([]tasks.Item, error) { <-release; return nil, nil }
		d.EnvScan = func() (envdoctor.Report, error) { <-release; return envdoctor.Report{}, nil }
		d.PrivacyList = func() ([]privacy.TweakView, error) { <-release; return nil, nil }
		d.HealthHistory = func(ctx context.Context) (healthhistory.View, error) { <-release; return healthhistory.View{}, nil }
	})
	cases := map[string]string{
		"winforge_health_scan": `{}`, "winforge_storage_scan": `{}`, "winforge_storage_biggest": `{"root":"C:\\Data","kind":"files"}`,
		"winforge_storage_projects": `{"roots":["C:\\Data"]}`, "winforge_startup_list": `{}`, "winforge_services_list": `{}`,
		"winforge_tasks_list": `{}`, "winforge_env_scan": `{}`, "winforge_privacy_list": `{}`, "winforge_health_history": `{}`,
	}
	for n, a := range cases {
		t.Run(n, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(30*time.Millisecond, cancel)
			start := time.Now()
			res := h.callCtx(ctx, n, a, nil)
			if !res.IsError || !strings.Contains(res.Text, "cancelled") {
				t.Errorf("result %+v", res)
			}
			if time.Since(start) > 3*time.Second {
				t.Errorf("took %v to stop", time.Since(start))
			}
		})
	}
	// Already cancelled: nothing runs.
	h2 := newHarness(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := h2.callCtx(ctx, "winforge_battery", `{}`, nil)
	if !res.IsError || len(h2.rec.got()) != 0 {
		t.Errorf("pre-cancelled: %+v %v", res, h2.rec.got())
	}
}

func TestProgressIsRedacted(t *testing.T) {
	h := newHarness(t, nil)
	var msgs []string
	h.callCtx(context.Background(), "winforge_storage_biggest", `{"root":"C:\\Users\\sebss\\Videos","kind":"files"}`, func(d, tot float64, msg string) { msgs = append(msgs, msg) })
	if len(msgs) == 0 {
		t.Fatal("no progress")
	}
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m), testUser) {
			t.Errorf("progress leaked: %q", m)
		}
	}
}

// Without an Env.Redact the tools still remove the user's name and home folder.
func TestFallbackRedaction(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home folder")
	}
	r := redactor(&mcpkit.Env{})
	in := "file at " + home + `\x\y`
	if out := r(in); strings.Contains(strings.ToLower(out), strings.ToLower(home)) {
		t.Errorf("home survived: %q", out)
	}
}

func TestReadHistoryFileDoesNotTouchDamagedFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.json")
	v, err := readHistoryFile(missing)
	if err != nil || len(v.Snapshots) != 0 {
		t.Errorf("missing: %v %v", v, err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("created the file")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err = readHistoryFile(bad)
	if err != nil || len(v.Snapshots) != 0 {
		t.Errorf("damaged: %v %v", v, err)
	}
	if b, err := os.ReadFile(bad); err != nil || string(b) != "{not json" {
		t.Errorf("damaged file changed: %q %v", b, err)
	}
	if _, err := os.Stat(bad + ".damaged"); err == nil {
		t.Error("renamed the damaged file")
	}
	good := filepath.Join(dir, "ok.json")
	f := healthhistory.File{Version: 1, Snapshots: []healthhistory.Snapshot{{At: fixedNow, Score: 10, Total: 20, Areas: map[string]int{}}}}
	b, _ := json.Marshal(f)
	if err := os.WriteFile(good, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err = readHistoryFile(good); err != nil || len(v.Snapshots) != 1 {
		t.Errorf("good: %v %v", v, err)
	}
	// A newer file reads as empty and stays.
	b, _ = json.Marshal(healthhistory.File{Version: 99})
	newer := filepath.Join(dir, "new.json")
	_ = os.WriteFile(newer, b, 0o600)
	if v, _ = readHistoryFile(newer); len(v.Snapshots) != 0 {
		t.Errorf("newer file: %v", v)
	}
}

func TestReadOnlyRunner(t *testing.T) {
	refused := [][]string{
		{"powershell.exe", "-Command", "Clear-RecycleBin"}, {"cmd.exe", "/C", "rd", "/s", "/q", `C:\Windows.old`},
		{"takeown.exe", "/F", "x"}, {"icacls.exe", "x"}, {"docker", "builder", "prune", "--all", "--force"},
		{"docker", "image", "prune", "--force"}, {"docker", "system", "prune"}, {"docker"}, {"docker", "rm", "x"},
	}
	for _, c := range refused {
		if allowedCommand(c[0], c[1:]) {
			t.Errorf("allowed %v", c)
		}
		if _, err := (readOnlyRunner{}).Run(context.Background(), c[0], c[1:]...); err == nil {
			t.Errorf("ran %v", c)
		}
	}
	for _, c := range [][]string{{"docker", "system", "df", "--format", "{{json .}}"}, {"docker", "images", "--filter", "dangling=true", "--format", "{{.Size}}"}} {
		if !allowedCommand(c[0], c[1:]) {
			t.Errorf("refused the read command %v", c)
		}
	}
}

// The package must not contain anything that writes: this scans the non-test sources.
func TestSourcesHaveNoWrites(t *testing.T) {
	forbidden := []string{"os.WriteFile", "os.Create", "os.Remove", "os.Rename", "os.Mkdir", "os.Chmod", "os.Truncate", "exec.Command", `/registry"`, `"syscall"`, `golang.org/x/sys`, "RemoveAll", "SetEnv", "Setenv"}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range forbidden {
			if strings.Contains(string(b), bad) {
				t.Errorf("%s contains %s", f, bad)
			}
		}
	}
}

// A Unix-style absolute path is only accepted where Windows is not the platform, so CI on Linux can use
// temporary folders while a Windows PC never sees one.
func TestUnixPathsOnlyOffWindows(t *testing.T) {
	_, err := checkFolder("/etc", false)
	if onWindows && err == nil {
		t.Error("/etc accepted on Windows")
	}
	if !onWindows && err != nil {
		t.Errorf("/etc refused off Windows: %v", err)
	}
}

func TestCapped(t *testing.T) {
	if out, cut := capped([]int(nil), 5); out == nil || len(out) != 0 || cut {
		t.Errorf("nil: %v %v", out, cut)
	}
	if out, cut := capped([]int{1, 2, 3}, 3); len(out) != 3 || cut {
		t.Errorf("exact: %v %v", out, cut)
	}
	if out, cut := capped([]int{1, 2, 3}, 2); len(out) != 2 || !cut {
		t.Errorf("cut: %v %v", out, cut)
	}
	if short("héllo wörld", 5) != "héllo..." {
		t.Errorf("short: %q", short("héllo wörld", 5))
	}
}
