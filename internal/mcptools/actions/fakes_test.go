package actions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// Nothing in these tests touches the real registry, disk, services, tasks or winget: every backend is a fake, and the
// fakes record what was read and what was written so a test can prove that prepare only reads.

const planted = "sebas" // the user name planted in paths: no summary or result may contain it

var fixedNow = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// planStore is a fake mcpkit.PlanStore: ids are unique, a plan is taken once.
type planStore struct {
	mu    sync.Mutex
	plans map[string]mcpkit.Plan
	n     int
	puts  int
}

func newPlanStore() *planStore { return &planStore{plans: map[string]mcpkit.Plan{}} }

func (s *planStore) Put(p mcpkit.Plan) (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	s.puts++
	id := fmt.Sprintf("plan-%d", s.n)
	s.plans[id] = p
	return id, fixedNow.Add(15 * time.Minute)
}

func (s *planStore) Take(id string) (mcpkit.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if !ok {
		return mcpkit.Plan{}, mcpkit.ErrPlanUnknown
	}
	delete(s.plans, id)
	return p, nil
}

// fakeExec is a fake install.Executor: it records the recipe each call runs (found by a marker in the
// script) and never starts a process.
type fakeExec struct {
	mu      sync.Mutex
	ran     []string
	codes   map[string]int // exit code by recipe id; default 0
	onRun   func(id string)
	scripts int
}

var markRe = regexp.MustCompile(`MARK:([a-z0-9-]+)`)

func (f *fakeExec) Run(ctx context.Context, name string, args []string, onLine func(string)) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts++
	raw, _ := base64.StdEncoding.DecodeString(args[len(args)-1])
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	m := markRe.FindStringSubmatch(string(utf16.Decode(u)))
	id := "?"
	if m != nil {
		id = m[1]
	}
	f.ran = append(f.ran, id)
	onLine("output of " + id + ` in C:\Users\` + planted + `\x`)
	if f.onRun != nil {
		f.onRun(id)
	}
	return f.codes[id], nil
}

func (f *fakeExec) recipes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ran...)
}

func testCatalog() *catalog.Catalog {
	c := &catalog.Catalog{Recipes: map[string]*catalog.Recipe{}}
	for _, id := range []string{
		"repair-restore-point", "repair-windows-files", "feature-wsl-on", "feature-wsl-off", "feature-vmp-on", "feature-vmp-off",
		"feature-hyperv-on", "feature-hyperv-off", "feature-sandbox-on", "feature-sandbox-off", "feature-netfx3-on", "feature-netfx3-off",
		"optimize-trim", "optimize-defrag", "optimize-component-cleanup", "optimize-hibernate-off",
	} {
		c.Recipes[id] = &catalog.Recipe{ID: id, Description: "does " + id, PowerShell: "# MARK:" + id, Admin: true}
	}
	return c
}

// testEnv is an Env whose Redact hides the planted user name, and whose elevation can be switched.
func testEnv(t *testing.T, elevated *bool, ex *fakeExec) *mcpkit.Env {
	t.Helper()
	cat := testCatalog()
	lock := t.TempDir() + "/run.lock"
	r := strings.NewReplacer(`C:\Users\`+planted, `C:\Users\<user>`, "/home/"+planted, "/home/<user>", planted, "<user>")
	env := &mcpkit.Env{
		Version:  "test",
		Catalog:  func() (*catalog.Catalog, error) { return cat, nil },
		Elevated: func() bool { return *elevated },
		LockPath: func() (string, error) { return lock, nil },
		Now:      func() time.Time { return fixedNow },
		Redact:   r.Replace,
	}
	if ex != nil {
		env.Exec = ex
	}
	return env
}

// harness drives the tools the way the server does.
type harness struct {
	t     *testing.T
	tools map[string]mcpkit.Tool
	store *planStore
	env   *mcpkit.Env
	prog  []string
}

func newHarness(t *testing.T, env *mcpkit.Env, d Deps) *harness {
	t.Helper()
	h := &harness{t: t, tools: map[string]mcpkit.Tool{}, store: newPlanStore(), env: env}
	for _, tool := range ToolsWith(env, d) {
		h.tools[tool.Name] = tool
	}
	return h
}

func (h *harness) callCtx(ctx context.Context, c *mcpkit.Call) *mcpkit.Call {
	c.Env, c.Plans = h.env, h.store
	c.Progress = func(done, total float64, msg string) { h.prog = append(h.prog, msg) }
	return c
}

func (h *harness) call(name, args string) (*mcpkit.Result, error) {
	h.t.Helper()
	tool, ok := h.tools[name]
	if !ok {
		h.t.Fatalf("no tool %s", name)
	}
	return tool.Handler(context.Background(), h.callCtx(context.Background(), &mcpkit.Call{Args: json.RawMessage(args)}))
}

// prepare expects a plan to be stored and returns what the tool answered.
func (h *harness) prepare(name, args string) prepared {
	h.t.Helper()
	before := h.store.puts
	res, err := h.call(name, args)
	if err != nil || res == nil || res.IsError {
		h.t.Fatalf("%s %s: err=%v res=%+v", name, args, err, res)
	}
	if h.store.puts != before+1 {
		h.t.Fatalf("%s stored %d plans, want 1", name, h.store.puts-before)
	}
	p, ok := res.Structured.(prepared)
	if !ok {
		h.t.Fatalf("not a prepared result: %T", res.Structured)
	}
	if p.PlanID == "" || p.Kind == "" || p.ExpiresAt == "" || p.Summary == nil {
		h.t.Fatalf("incomplete result %+v", p)
	}
	assertNoUser(h.t, res.Text)
	return p
}

// refused expects the request to be answered with an error and nothing stored; it returns the text.
func (h *harness) refused(name, args string) string {
	h.t.Helper()
	before := h.store.puts
	res, err := h.call(name, args)
	if err != nil {
		h.t.Fatalf("%s %s: unexpected error %v", name, args, err)
	}
	if res == nil || !res.IsError {
		h.t.Fatalf("%s %s: expected a refusal, got %+v", name, args, res)
	}
	if h.store.puts != before {
		h.t.Fatalf("%s %s: stored a plan on a refusal", name, args)
	}
	assertNoUser(h.t, res.Text)
	return res.Text
}

// run applies a plan like winforge_apply_plan: take it once, run it.
func (h *harness) run(id string) (*mcpkit.Result, error) {
	h.t.Helper()
	return h.runCtx(context.Background(), id)
}

func (h *harness) runCtx(ctx context.Context, id string) (*mcpkit.Result, error) {
	h.t.Helper()
	plan, err := h.store.Take(id)
	if err != nil {
		h.t.Fatalf("take %s: %v", id, err)
	}
	return plan.Run(ctx, h.callCtx(ctx, &mcpkit.Call{}))
}

// ran expects the plan to run and answer without an error.
func (h *harness) ran(id string) *mcpkit.Result {
	h.t.Helper()
	res, err := h.run(id)
	if err != nil || res == nil {
		h.t.Fatalf("run %s: err=%v res=%v", id, err, res)
	}
	assertNoUser(h.t, res.Text)
	return res
}

// ranRefused expects the plan to refuse at run time.
func (h *harness) ranRefused(id string) string {
	h.t.Helper()
	res, err := h.run(id)
	if err != nil || res == nil || !res.IsError {
		h.t.Fatalf("run %s: expected a refusal, got err=%v res=%+v", id, err, res)
	}
	assertNoUser(h.t, res.Text)
	return res.Text
}

func assertNoUser(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(strings.ToLower(text), planted) {
		t.Fatalf("the user name leaked:\n%s", text)
	}
}

// asMap turns any summary or result into generic JSON for inspection.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func yes() *bool { b := true; return &b }

func featuresFixed(list ...winfeatures.Feature) func() []winfeatures.Feature {
	return func() []winfeatures.Feature { return list }
}
