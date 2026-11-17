package sysapps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/system"
)

var fixedNow = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// fixtureCatalog is a small catalog with an admin app, a requirement, recipes, two profiles, an app the
// end-of-support table knows (python-3-10) and 250 bulk apps for truncation tests.
func fixtureCatalog() *catalog.Catalog {
	cat := &catalog.Catalog{
		Apps: map[string]*catalog.App{
			"alpha":       {ID: "alpha", Name: "Alpha Editor", Category: "Editors", Publisher: "Acme", Winget: "Acme.Alpha", Description: "A text editor", Tags: []string{"text", "code"}, OpenSource: true},
			"beta":        {ID: "beta", Name: "Beta Server", Category: "Servers", Publisher: "Bravo Inc", Winget: "Bravo.Beta", Description: "A server", Admin: true, Requires: []string{"alpha"}},
			"gamma":       {ID: "gamma", Name: "Gamma Tool", Category: "Tools", Publisher: "Gamma LLC", Winget: "Gamma.Tool", Description: "A tool"},
			"delta":       {ID: "delta", Name: "Delta Player", Category: "Media", Publisher: "Delta Co", Winget: "Delta.Player", Description: "A player", Recipes: []string{"rec-d"}},
			"python-3-10": {ID: "python-3-10", Name: "Python 3.10", Category: "Dev", Publisher: "PSF", Winget: "Python.Python.3.10", Description: "Python"},
		},
		Recipes: map[string]*catalog.Recipe{
			"rec-a": {ID: "rec-a", Description: "Tweak A", PowerShell: "Write-Output a", After: []string{"alpha"}},
			"rec-d": {ID: "rec-d", Description: "Tweak D", PowerShell: "Write-Output d"},
		},
		Profiles: map[string]*catalog.Profile{
			"starter":   {ID: "starter", Name: "Starter", Kind: "base", Description: "The basics", Apps: []catalog.ProfileApp{{ID: "alpha"}, {ID: "gamma", Version: "1.2.3"}}, Recipes: []string{"rec-a"}},
			"adminpack": {ID: "adminpack", Name: "Admin pack", Kind: "dev", Apps: []catalog.ProfileApp{{ID: "beta"}}},
		},
	}
	for i := 1; i <= 250; i++ {
		id := fmt.Sprintf("bulk-%03d", i)
		cat.Apps[id] = &catalog.App{ID: id, Name: fmt.Sprintf("Bulk App %03d", i), Category: "Bulk", Publisher: "Bulk Inc", Winget: fmt.Sprintf("Bulk.App%03d", i), Description: "bulk"}
	}
	return cat
}

// fakeExec answers winget and PowerShell calls without touching the machine and keeps the fake inventory in step.
type fakeExec struct {
	mu    sync.Mutex
	calls []string
	exit  func(name string, args []string) int
	block bool
	err   error
	apply func(name string, args []string)
}

func (f *fakeExec) Run(ctx context.Context, name string, args []string, onLine func(string)) (int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.mu.Unlock()
	if name == "winget" && len(args) == 1 && args[0] == "--version" {
		onLine("v1.9.25180")
		return 0, nil
	}
	if f.err != nil {
		return -1, f.err
	}
	if f.block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	onLine("working")
	code := 0
	if f.exit != nil {
		code = f.exit(name, args)
	}
	if f.apply != nil && (code == 0 || code == 3010) {
		f.apply(name, args)
	}
	return code, nil
}

func (f *fakeExec) wingetCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "winget") && !strings.HasPrefix(c, "winget --version") {
			out = append(out, c)
		}
	}
	return out
}

type memPlans struct {
	mu sync.Mutex
	m  map[string]mcpkit.Plan
	n  int
}

func (p *memPlans) Put(pl mcpkit.Plan) (string, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]mcpkit.Plan{}
	}
	p.n++
	id := fmt.Sprintf("plan-%d", p.n)
	p.m[id] = pl
	return id, fixedNow.Add(30 * time.Minute)
}

func (p *memPlans) Take(id string) (mcpkit.Plan, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pl, ok := p.m[id]
	if !ok {
		return mcpkit.Plan{}, mcpkit.ErrPlanUnknown
	}
	delete(p.m, id)
	return pl, nil
}

func (p *memPlans) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.m)
}

type rig struct {
	t       *testing.T
	root    string
	appData string
	cat     *catalog.Catalog
	env     *mcpkit.Env
	deps    Deps
	exec    *fakeExec
	plans   *memPlans
	tools   map[string]mcpkit.Tool

	mu       sync.Mutex
	snaps    int
	inv      map[string]string // lower-cased winget id -> version
	snapErr  error
	elev     bool
	winget   bool
	upgrades []system.Upgrade
	upErr    error
	upCalls  int
	lock     string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	root := t.TempDir()
	r := &rig{t: t, root: root, appData: filepath.Join(root, "WinForge"), cat: fixtureCatalog(), exec: &fakeExec{}, plans: &memPlans{},
		inv: map[string]string{}, winget: true, lock: filepath.Join(root, "run.lock")}
	r.env = &mcpkit.Env{
		Version: "9.9.9-test",
		Catalog: func() (*catalog.Catalog, error) { return r.cat, nil },
		Snapshot: func(context.Context) (system.Inventory, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.snaps++
			w := map[string]string{}
			for k, v := range r.inv {
				w[k] = v
			}
			return system.Inventory{Winget: w, LookPath: func(string) bool { return false }, Exists: func(string) bool { return false }}, r.snapErr
		},
		Exec:     r.exec,
		Elevated: func() bool { return r.elev },
		LockPath: func() (string, error) { return r.lock, nil },
		Now:      func() time.Time { return fixedNow },
		AppData:  func() (string, error) { return r.appData, nil },
		Redact:   func(s string) string { return strings.ReplaceAll(s, root, "<home>") },
	}
	r.deps = Deps{
		LookPath: func(string) (string, error) {
			if !r.winget {
				return "", errors.New("not found")
			}
			return filepath.Join(root, "bin", "winget.exe"), nil
		},
		Upgrades: func(context.Context) ([]system.Upgrade, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.upCalls++
			return r.upgrades, r.upErr
		},
	}
	r.exec.apply = func(name string, args []string) {
		if name != "winget" || len(args) < 3 {
			return
		}
		var id string
		for i, a := range args {
			if a == "--id" && i+1 < len(args) {
				id = strings.ToLower(args[i+1])
			}
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		switch args[0] {
		case "install", "upgrade":
			r.inv[id] = "2.0.0"
		case "uninstall":
			delete(r.inv, id)
		}
	}
	r.build()
	return r
}

func (r *rig) build() {
	r.tools = map[string]mcpkit.Tool{}
	for _, tl := range ToolsWith(r.env, r.deps) {
		r.tools[tl.Name] = tl
	}
}

// install marks catalog apps as installed (id -> version).
func (r *rig) install(pairs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i+1 < len(pairs); i += 2 {
		r.inv[strings.ToLower(r.cat.Apps[pairs[i]].Winget)] = pairs[i+1]
	}
}

func (r *rig) snapshots() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snaps
}

func (r *rig) call(name, args string) *mcpkit.Result {
	r.t.Helper()
	return r.callCtx(context.Background(), name, args)
}

func (r *rig) callCtx(ctx context.Context, name, args string) *mcpkit.Result {
	r.t.Helper()
	tl, ok := r.tools[name]
	if !ok {
		r.t.Fatalf("no tool %s", name)
	}
	res, err := tl.Handler(ctx, &mcpkit.Call{Args: json.RawMessage(args), Env: r.env, Plans: r.plans})
	if err != nil {
		r.t.Fatalf("%s returned a Go error: %v", name, err)
	}
	if res == nil {
		r.t.Fatalf("%s returned nil", name)
	}
	return res
}

// ok calls a tool and decodes its JSON answer, failing on a tool error.
func (r *rig) ok(name, args string) map[string]any {
	r.t.Helper()
	res := r.call(name, args)
	if res.IsError {
		r.t.Fatalf("%s %s: unexpected error: %s", name, args, res.Text)
	}
	return decodeText(r.t, res)
}

func (r *rig) fails(name, args, want string) {
	r.t.Helper()
	res := r.call(name, args)
	if !res.IsError {
		r.t.Fatalf("%s %s: expected an error, got %s", name, args, res.Text)
	}
	if want != "" && !strings.Contains(res.Text, want) {
		r.t.Fatalf("%s %s: error %q does not mention %q", name, args, res.Text, want)
	}
}

func decodeText(t *testing.T, res *mcpkit.Result) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Text), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, res.Text)
	}
	return m
}

func list(m map[string]any, key string) []any {
	l, _ := m[key].([]any)
	return l
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func boolean(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

func num(m map[string]any, key string) int {
	f, _ := m[key].(float64)
	return int(f)
}

func ids(l []any, key string) []string {
	var out []string
	for _, x := range l {
		out = append(out, str(obj(x), key))
	}
	return out
}

// apply takes a stored plan and runs it like winforge_apply_plan does.
func (r *rig) apply(ctx context.Context, planID string) *mcpkit.Result {
	r.t.Helper()
	pl, err := r.plans.Take(planID)
	if err != nil {
		r.t.Fatalf("take %s: %v", planID, err)
	}
	res, err := pl.Run(ctx, &mcpkit.Call{Env: r.env, Plans: r.plans})
	if err != nil {
		r.t.Fatalf("run: %v", err)
	}
	return res
}
