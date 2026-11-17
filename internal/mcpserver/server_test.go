package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/diag"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/sched"
)

// ---- fakes ----

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// harness is a fake client talking to a real Server through pipes.
type harness struct {
	t      *testing.T
	in     *io.PipeWriter
	clock  *fakeClock
	store  *PlanStore
	env    *mcpkit.Env
	served chan error

	mu    sync.Mutex
	resp  map[string]map[string]json.RawMessage
	notes []map[string]json.RawMessage
	raw   bytes.Buffer
}

type setup struct {
	mode     Mode
	groups   [][]mcpkit.Tool
	redact   func(string) string
	elevated bool
	noLock   bool
	readTO   time.Duration
	actTO    time.Duration
}

func startServer(t *testing.T, s setup) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}, served: make(chan error, 1),
		resp: map[string]map[string]json.RawMessage{}}
	h.store = NewPlanStore(10*time.Minute, h.clock.Now)
	lock := filepath.Join(t.TempDir(), "run.lock")
	h.env = &mcpkit.Env{
		Version:  "test",
		Elevated: func() bool { return s.elevated },
		Now:      h.clock.Now,
		Redact:   s.redact,
	}
	if !s.noLock {
		h.env.LockPath = func() (string, error) { return lock, nil }
	}
	groups := append([][]mcpkit.Tool{Builtin(h.store)}, s.groups...)
	reg, err := NewRegistry(groups...)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Mode: s.mode, Registry: reg, Store: h.store, Env: h.env, Version: "9.9.9",
		ReadTimeout: s.readTO, ActTimeout: s.actTO, Log: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h.in = inW
	go func() {
		h.served <- srv.Serve(context.Background(), inR, outW)
		outW.Close()
	}()
	go func() {
		buf := make([]byte, 0, 64<<10)
		tmp := make([]byte, 64<<10)
		for {
			n, err := outR.Read(tmp)
			buf = append(buf, tmp[:n]...)
			for {
				i := bytes.IndexByte(buf, '\n')
				if i < 0 {
					break
				}
				line := buf[:i]
				buf = buf[i+1:]
				h.mu.Lock()
				h.raw.Write(line)
				h.raw.WriteByte('\n')
				var m map[string]json.RawMessage
				if json.Unmarshal(line, &m) != nil {
					t.Errorf("stdout carried a line that is not JSON: %q", line)
				} else if id, ok := m["id"]; ok && (m["result"] != nil || m["error"] != nil) {
					h.resp[string(id)] = m
				} else {
					h.notes = append(h.notes, m)
				}
				h.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { h.in.Close() })
	return h
}

func (h *harness) send(v any) {
	h.t.Helper()
	b, _ := json.Marshal(v)
	h.sendRaw(string(b))
}

func (h *harness) sendRaw(line string) {
	h.t.Helper()
	if _, err := h.in.Write([]byte(line + "\n")); err != nil {
		h.t.Fatalf("write to server: %v", err)
	}
}

func (h *harness) request(id any, method string, params any) {
	h.t.Helper()
	m := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	h.send(m)
}

func (h *harness) await(id any, timeout time.Duration) map[string]json.RawMessage {
	h.t.Helper()
	key, _ := json.Marshal(id)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		m, ok := h.resp[string(key)]
		h.mu.Unlock()
		if ok {
			return m
		}
		time.Sleep(2 * time.Millisecond)
	}
	h.t.Fatalf("no response to id %s within %s", key, timeout)
	return nil
}

func (h *harness) noResponse(id any, wait time.Duration) {
	h.t.Helper()
	time.Sleep(wait)
	key, _ := json.Marshal(id)
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.resp[string(key)]; ok {
		h.t.Fatalf("unexpected response to id %s: %s", key, m["result"])
	}
}

func (h *harness) call(id any, method string, params any) map[string]json.RawMessage {
	h.t.Helper()
	h.request(id, method, params)
	return h.await(id, 5*time.Second)
}

func (h *harness) errCode(m map[string]json.RawMessage) int {
	h.t.Helper()
	var e struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(m["error"], &e) != nil || m["error"] == nil {
		h.t.Fatalf("expected a JSON-RPC error, got %s", m["result"])
	}
	return e.Code
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Structured json.RawMessage `json:"structuredContent"`
	IsError    bool            `json:"isError"`
}

func (r toolResult) text() string {
	var b strings.Builder
	for _, c := range r.Content {
		b.WriteString(c.Text)
	}
	return b.String()
}

func (h *harness) tool(id any, name string, args any) toolResult {
	h.t.Helper()
	return h.parseTool(h.call(id, "tools/call", map[string]any{"name": name, "arguments": args}))
}

func (h *harness) parseTool(m map[string]json.RawMessage) toolResult {
	h.t.Helper()
	if m["error"] != nil {
		h.t.Fatalf("protocol error instead of a tool result: %s", m["error"])
	}
	var r toolResult
	if err := json.Unmarshal(m["result"], &r); err != nil {
		h.t.Fatal(err)
	}
	return r
}

func (h *harness) init() {
	h.t.Helper()
	m := h.call("init", "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1"}})
	if m["error"] != nil {
		h.t.Fatalf("initialize failed: %s", m["error"])
	}
	h.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

func (h *harness) names(id any) []string {
	h.t.Helper()
	m := h.call(id, "tools/list", nil)
	var r struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(m["result"], &r); err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, t := range r.Tools {
		out = append(out, t.Name)
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---- fake tools ----

func readTool(name string, h func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error)) mcpkit.Tool {
	return mcpkit.Tool{Name: name, Title: name, Description: "test " + name, InputSchema: mcpkit.NoArgs, Class: mcpkit.Read, Handler: h}
}

func planID(t *testing.T, r toolResult) string {
	t.Helper()
	var s struct {
		PlanID string `json:"planId"`
	}
	if err := json.Unmarshal(r.Structured, &s); err != nil || s.PlanID == "" {
		t.Fatalf("no plan id in %s", r.text())
	}
	return s.PlanID
}

// ---- tests ----

func TestInitializeNegotiatesVersion(t *testing.T) {
	cases := map[string]string{
		"2025-06-18": "2025-06-18", "2025-03-26": "2025-03-26", "2024-11-05": "2024-11-05",
		"2099-01-01": "2025-06-18", "": "2025-06-18",
	}
	n := 0
	for ask, want := range cases {
		n++
		h := startServer(t, setup{})
		m := h.call(n, "initialize", map[string]any{"protocolVersion": ask, "clientInfo": map[string]any{"name": "x"}})
		var r struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct{ Name, Version string }
			Capabilities    struct {
				Tools struct {
					ListChanged *bool `json:"listChanged"`
				} `json:"tools"`
			}
			Instructions string
		}
		if err := json.Unmarshal(m["result"], &r); err != nil {
			t.Fatal(err)
		}
		if r.ProtocolVersion != want {
			t.Errorf("client asked %q: got %q, want %q", ask, r.ProtocolVersion, want)
		}
		if r.ServerInfo.Name != "winforge" || r.ServerInfo.Version != "9.9.9" {
			t.Errorf("serverInfo = %+v", r.ServerInfo)
		}
		if r.Capabilities.Tools.ListChanged == nil || *r.Capabilities.Tools.ListChanged {
			t.Errorf("capabilities.tools.listChanged should be false")
		}
		for _, w := range []string{"model", "approved", "read tools"} {
			if !strings.Contains(r.Instructions, w) {
				t.Errorf("instructions lack %q", w)
			}
		}
	}
}

func TestPingAndNotInitialized(t *testing.T) {
	h := startServer(t, setup{})
	if m := h.call(1, "ping", nil); string(m["result"]) != "{}" {
		t.Errorf("ping = %s", m["result"])
	}
	if c := h.errCode(h.call(2, "tools/list", nil)); c != codeInvalidRequest {
		t.Errorf("tools/list before initialize: code %d", c)
	}
	h.init()
	if c := h.errCode(h.call(3, "nope/nothing", nil)); c != codeMethodNotFound {
		t.Errorf("unknown method: code %d", c)
	}
	// An unknown notification gets no answer and does not hurt.
	h.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/whatever"})
	h.call(4, "ping", nil)
}

func TestToolsListPerMode(t *testing.T) {
	for _, tc := range []struct {
		mode        Mode
		prepare     bool
		act         bool
		wantFakeAct bool
	}{{ModeRead, false, false, false}, {ModePlan, true, false, false}, {ModeFull, true, true, true}} {
		h := startServer(t, setup{mode: tc.mode, groups: [][]mcpkit.Tool{{
			readTool("fake_read", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return mcpkit.Text("r"), nil }),
			{Name: "fake_act", Title: "a", Description: "a", Class: mcpkit.Act, Handler: func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return mcpkit.Text("a"), nil }},
		}}})
		h.init()
		names := h.names("l")
		if !has(names, "fake_read") || !has(names, ToolListPlans) {
			t.Errorf("%s: read tools missing: %v", tc.mode, names)
		}
		if has(names, ToolDiscardPlan) != tc.prepare {
			t.Errorf("%s: discard listed = %v", tc.mode, has(names, ToolDiscardPlan))
		}
		if has(names, ToolApplyPlan) != tc.act || has(names, "fake_act") != tc.wantFakeAct {
			t.Errorf("%s: act tools listed wrongly: %v", tc.mode, names)
		}
	}
}

func TestToolsListAnnotations(t *testing.T) {
	h := startServer(t, setup{mode: ModeFull})
	h.init()
	m := h.call("l", "tools/list", nil)
	var r struct {
		Tools []struct {
			Name        string
			Title       string
			Description string
			InputSchema map[string]any
			Annotations map[string]bool
		}
	}
	if err := json.Unmarshal(m["result"], &r); err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]bool{}
	for _, tl := range r.Tools {
		by[tl.Name] = tl.Annotations
		if tl.Title == "" || tl.Description == "" || tl.InputSchema["type"] != "object" {
			t.Errorf("%s: incomplete listing", tl.Name)
		}
		for _, k := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
			if _, ok := tl.Annotations[k]; !ok {
				t.Errorf("%s: annotation %s missing", tl.Name, k)
			}
		}
		if tl.Annotations["openWorldHint"] {
			t.Errorf("%s: openWorldHint must be false", tl.Name)
		}
	}
	a := by[ToolApplyPlan]
	if a["readOnlyHint"] || !a["destructiveHint"] || a["idempotentHint"] {
		t.Errorf("apply annotations = %v", a)
	}
	if l := by[ToolListPlans]; !l["readOnlyHint"] || l["destructiveHint"] {
		t.Errorf("list annotations = %v", l)
	}
}

func TestToolCallResultsAndErrors(t *testing.T) {
	h := startServer(t, setup{groups: [][]mcpkit.Tool{{
		readTool("fake_json", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) {
			return mcpkit.JSON(map[string]any{"apps": []string{"a", "b"}}), nil
		}),
		readTool("fake_fail", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return nil, fmt.Errorf("it broke") }),
		readTool("fake_isfail", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return mcpkit.Fail("nope"), nil }),
		readTool("fake_list", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return mcpkit.JSON([]int{1, 2}), nil }),
		readTool("fake_panic", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { panic("boom") }),
	}}})
	h.init()
	r := h.tool(1, "fake_json", map[string]any{})
	if r.IsError || !strings.Contains(r.text(), `"apps"`) || !strings.Contains(string(r.Structured), `["a","b"]`) {
		t.Errorf("json tool: %+v / %s", r, r.Structured)
	}
	if r.Content[0].Type != "text" {
		t.Errorf("content type = %q", r.Content[0].Type)
	}
	if r := h.tool(2, "fake_fail", nil); !r.IsError || !strings.Contains(r.text(), "it broke") {
		t.Errorf("handler error: %+v", r)
	}
	if r := h.tool(3, "fake_isfail", nil); !r.IsError || r.text() != "nope" {
		t.Errorf("Fail result: %+v", r)
	}
	if r := h.tool(4, "fake_list", nil); !strings.HasPrefix(string(r.Structured), `{"result"`) {
		t.Errorf("non-object structured content must be wrapped: %s", r.Structured)
	}
	if r := h.tool(5, "fake_panic", nil); !r.IsError || strings.Contains(r.text(), "boom") == false && r.text() == "" {
		t.Errorf("panic: %+v", r)
	}
	// The server is still alive after a panic.
	if r := h.tool(6, "fake_json", nil); r.IsError {
		t.Errorf("server unusable after a panic")
	}
	// Protocol errors.
	if c := h.errCode(h.call(7, "tools/call", map[string]any{"name": "does_not_exist"})); c != codeInvalidParams {
		t.Errorf("unknown tool code %d", c)
	}
	if c := h.errCode(h.call(8, "tools/call", map[string]any{})); c != codeInvalidParams {
		t.Errorf("no name code %d", c)
	}
	if c := h.errCode(h.call(9, "tools/call", map[string]any{"name": "fake_json", "arguments": []int{1}})); c != codeInvalidParams {
		t.Errorf("array arguments code %d", c)
	}
	if c := h.errCode(h.call(10, "tools/call", "not an object")); c != codeInvalidParams {
		t.Errorf("string params code %d", c)
	}
}

func TestModeRefusal(t *testing.T) {
	var runs atomic.Int32
	act := mcpkit.Tool{Name: "fake_act", Title: "a", Description: "a", Class: mcpkit.Act, Handler: func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) {
		runs.Add(1)
		return mcpkit.Text("ran"), nil
	}}
	for _, tc := range []struct {
		mode Mode
		tool string
		flag string
	}{{ModeRead, ToolDiscardPlan, "--mode plan"}, {ModeRead, ToolApplyPlan, "--mode full"}, {ModePlan, ToolApplyPlan, "--mode full"}, {ModePlan, "fake_act", "--mode full"}} {
		h := startServer(t, setup{mode: tc.mode, groups: [][]mcpkit.Tool{{act}}})
		h.init()
		r := h.tool("c", tc.tool, map[string]any{"planId": "x"})
		if !r.IsError || !strings.Contains(r.text(), tc.flag) {
			t.Errorf("%s in %s: %+v", tc.tool, tc.mode, r)
		}
	}
	if runs.Load() != 0 {
		t.Errorf("a refused Act tool ran")
	}
}

func TestPrepareApplySingleUse(t *testing.T) {
	var runs atomic.Int32
	h := startWith(t, ModeFull, &runs, false, true)
	h.init()
	id := planID(t, h.tool(1, "fake_prepare", nil))
	if len(id) != 32 {
		t.Errorf("plan id %q is not 128 bits of hex", id)
	}
	if r := h.tool(2, ToolListPlans, nil); !strings.Contains(r.text(), id) || !strings.Contains(r.text(), "do a thing") {
		t.Errorf("list_plans: %s", r.text())
	}
	r := h.tool(3, ToolApplyPlan, map[string]any{"planId": id})
	if r.IsError || r.text() != "did the thing" || runs.Load() != 1 {
		t.Errorf("apply: %+v runs=%d", r, runs.Load())
	}
	if r := h.tool(4, ToolApplyPlan, map[string]any{"planId": id}); !r.IsError || runs.Load() != 1 {
		t.Errorf("second apply must be refused: %+v", r)
	}
	if r := h.tool(5, ToolApplyPlan, map[string]any{"planId": "00000000000000000000000000000000"}); !r.IsError {
		t.Errorf("unknown id must be refused")
	}
	if r := h.tool(6, ToolApplyPlan, map[string]any{"planId": id, "extra": 1}); !r.IsError {
		t.Errorf("unknown argument must be refused")
	}
	if r := h.tool(7, ToolListPlans, nil); strings.Contains(r.text(), id) {
		t.Errorf("a used plan is still listed")
	}
	// Discard.
	id2 := planID(t, h.tool(8, "fake_prepare", nil))
	if r := h.tool(9, ToolDiscardPlan, map[string]any{"planId": id2}); r.IsError {
		t.Errorf("discard: %+v", r)
	}
	if r := h.tool(10, ToolApplyPlan, map[string]any{"planId": id2}); !r.IsError || runs.Load() != 1 {
		t.Errorf("a discarded plan ran")
	}
	if r := h.tool(11, ToolDiscardPlan, map[string]any{"planId": id2}); !r.IsError {
		t.Errorf("discarding twice should say it is unknown")
	}
}

// startWith builds a harness whose groups include a prepare tool bound to the harness store.
func startWith(t *testing.T, mode Mode, runs *atomic.Int32, admin, elevated bool) *harness {
	t.Helper()
	prep := mcpkit.Tool{Name: "fake_prepare", Title: "p", Description: "prepare", InputSchema: mcpkit.NoArgs, Class: mcpkit.Prepare,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			id, exp := c.Plans.Put(mcpkit.Plan{Kind: "fake", Summary: map[string]any{"would": "do a thing"}, Admin: admin,
				Run: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
					runs.Add(1)
					c.Progress(1, 2, "halfway")
					c.Progress(2, 2, "done")
					return mcpkit.Text("did the thing"), nil
				}})
			return mcpkit.JSON(map[string]any{"planId": id, "expires": exp}), nil
		}}
	return startServer(t, setup{mode: mode, elevated: elevated, groups: [][]mcpkit.Tool{{prep}}})
}

func TestPlanExpiry(t *testing.T) {
	var runs atomic.Int32
	h := startWith(t, ModeFull, &runs, false, true)
	h.init()
	id := planID(t, h.tool(1, "fake_prepare", nil))
	h.clock.Add(10*time.Minute + time.Second)
	r := h.tool(2, ToolApplyPlan, map[string]any{"planId": id})
	if !r.IsError || runs.Load() != 0 || !strings.Contains(r.text(), "expired") {
		t.Errorf("expired plan: %+v runs=%d", r, runs.Load())
	}
	// Just before the TTL it still works.
	id = planID(t, h.tool(3, "fake_prepare", nil))
	h.clock.Add(9*time.Minute + 59*time.Second)
	if r := h.tool(4, ToolApplyPlan, map[string]any{"planId": id}); r.IsError || runs.Load() != 1 {
		t.Errorf("plan just inside its TTL: %+v", r)
	}
}

func TestAdminPlanNeedsElevation(t *testing.T) {
	var runs atomic.Int32
	h := startWith(t, ModeFull, &runs, true, false)
	h.init()
	id := planID(t, h.tool(1, "fake_prepare", nil))
	r := h.tool(2, ToolApplyPlan, map[string]any{"planId": id})
	if !r.IsError || runs.Load() != 0 || !strings.Contains(r.text(), "elevated") {
		t.Errorf("admin plan without elevation: %+v", r)
	}
	h2 := startWith(t, ModeFull, &runs, true, true)
	h2.init()
	id = planID(t, h2.tool(1, "fake_prepare", nil))
	if r := h2.tool(2, ToolApplyPlan, map[string]any{"planId": id}); r.IsError || runs.Load() != 1 {
		t.Errorf("admin plan when elevated: %+v", r)
	}
}

func TestProgressNotifications(t *testing.T) {
	var runs atomic.Int32
	h := startWith(t, ModeFull, &runs, false, true)
	h.init()
	id := planID(t, h.tool(1, "fake_prepare", nil))
	m := h.call(2, "tools/call", map[string]any{"name": ToolApplyPlan, "arguments": map[string]any{"planId": id},
		"_meta": map[string]any{"progressToken": "tok-1"}})
	if h.parseTool(m).IsError {
		t.Fatal("apply failed")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var got []float64
	for _, n := range h.notes {
		var method string
		_ = json.Unmarshal(n["method"], &method)
		if method != "notifications/progress" {
			continue
		}
		var p struct {
			Token    string  `json:"progressToken"`
			Progress float64 `json:"progress"`
			Total    float64 `json:"total"`
			Message  string  `json:"message"`
		}
		_ = json.Unmarshal(n["params"], &p)
		if p.Token != "tok-1" || p.Total != 2 || p.Message == "" {
			t.Errorf("progress params = %+v", p)
		}
		got = append(got, p.Progress)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("progress notifications = %v", got)
	}
}

func TestNoProgressWithoutToken(t *testing.T) {
	var runs atomic.Int32
	h := startWith(t, ModeFull, &runs, false, true)
	h.init()
	id := planID(t, h.tool(1, "fake_prepare", nil))
	h.tool(2, ToolApplyPlan, map[string]any{"planId": id})
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.notes) != 0 {
		t.Errorf("progress sent without a token: %v", h.notes)
	}
}

func TestCancellation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	h := startServer(t, setup{groups: [][]mcpkit.Tool{{
		readTool("fake_block", func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		}),
	}}})
	h.init()
	h.request(77, "tools/call", map[string]any{"name": "fake_block"})
	<-started
	h.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": 77, "reason": "user"}})
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("the handler's context was not cancelled")
	}
	h.noResponse(77, 100*time.Millisecond)
	h.call("p", "ping", nil) // still alive
}

func TestTimeout(t *testing.T) {
	h := startServer(t, setup{readTO: 60 * time.Millisecond, groups: [][]mcpkit.Tool{{
		readTool("fake_slow", func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}),
		readTool("fake_deaf", func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			time.Sleep(300 * time.Millisecond) // ignores its context
			return mcpkit.Text("late"), nil
		}),
	}}})
	h.init()
	for i, name := range []string{"fake_slow", "fake_deaf"} {
		r := h.tool(i+1, name, nil)
		if !r.IsError || !strings.Contains(r.text(), "did not finish") {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestActToolsRunOneAtATimeAndTakeTheRunLock(t *testing.T) {
	var active, maxActive atomic.Int32
	var lockSeen atomic.Bool
	act := func(name string) mcpkit.Tool {
		return mcpkit.Tool{Name: name, Title: name, Description: name, Class: mcpkit.Act,
			Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
				n := active.Add(1)
				for {
					m := maxActive.Load()
					if n <= m || maxActive.CompareAndSwap(m, n) {
						break
					}
				}
				if p, _ := c.Env.LockPath(); p != "" {
					if _, err := os.Stat(p); err == nil {
						lockSeen.Store(true)
					}
				}
				time.Sleep(60 * time.Millisecond)
				active.Add(-1)
				return mcpkit.Text("%s", name), nil
			}}
	}
	h := startServer(t, setup{mode: ModeFull, groups: [][]mcpkit.Tool{{act("fake_act1"), act("fake_act2"), act("fake_act3")}}})
	h.init()
	for i, n := range []string{"fake_act1", "fake_act2", "fake_act3"} {
		h.request(i, "tools/call", map[string]any{"name": n})
	}
	for i := 0; i < 3; i++ {
		if r := h.parseTool(h.await(i, 5*time.Second)); r.IsError {
			t.Errorf("act %d failed: %s", i, r.text())
		}
	}
	if maxActive.Load() != 1 {
		t.Errorf("Act tools overlapped: max concurrency %d", maxActive.Load())
	}
	if !lockSeen.Load() {
		t.Errorf("the run lock was not held while the Act tool ran")
	}
	p, _ := h.env.LockPath()
	if _, err := os.Stat(p); err == nil {
		t.Errorf("the run lock was not released")
	}

	// A lock held by someone else (apply, updates, the app) makes the tool refuse, plainly.
	release, err := sched.Lock{Path: p}.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	r := h.tool("busy", "fake_act1", nil)
	release()
	if !r.IsError || !strings.Contains(r.text(), "in progress") {
		t.Errorf("busy lock: %+v", r)
	}
	if r := h.tool("after", "fake_act1", nil); r.IsError {
		t.Errorf("the tool should work once the lock is free: %s", r.text())
	}
}

func TestReadToolsRunConcurrently(t *testing.T) {
	var active, maxActive atomic.Int32
	slow := readTool("fake_slow", func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		n := active.Add(1)
		for {
			m := maxActive.Load()
			if n <= m || maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
		active.Add(-1)
		return mcpkit.Text("ok"), nil
	})
	h := startServer(t, setup{groups: [][]mcpkit.Tool{{slow}}})
	h.init()
	for i := 0; i < 8; i++ {
		h.request(i, "tools/call", map[string]any{"name": "fake_slow"})
	}
	for i := 0; i < 8; i++ {
		h.await(i, 5*time.Second)
	}
	if m := maxActive.Load(); m < 2 || m > DefaultConcurrency {
		t.Errorf("max concurrent read calls = %d, want between 2 and %d", m, DefaultConcurrency)
	}
}

func TestGarbageInput(t *testing.T) {
	h := startServer(t, setup{groups: [][]mcpkit.Tool{{
		readTool("fake_echo", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return mcpkit.Text("echo"), nil }),
	}}})
	h.init()
	for _, line := range []string{
		"", "   ", "not json", "{", `{"jsonrpc":"2.0"}`, `{"jsonrpc":"1.0","id":1,"method":"ping"}`, `[1,2]`, `null`, `42`, `"str"`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`, `{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`,
		"\x00\x01\x02", `{"jsonrpc":"2.0","id":5,"result":{}}`,
	} {
		h.sendRaw(line)
	}
	// A BOM in front of a valid message is tolerated.
	h.sendRaw("\xef\xbb\xbf" + `{"jsonrpc":"2.0","id":"bom","method":"ping"}`)
	h.await("bom", 3*time.Second)
	if r := h.tool("after", "fake_echo", nil); r.text() != "echo" {
		t.Errorf("server unusable after garbage: %+v", r)
	}
	h.mu.Lock()
	out := h.raw.String()
	h.mu.Unlock()
	if !strings.Contains(out, `"code":-32700`) || !strings.Contains(out, `"code":-32600`) {
		t.Errorf("expected parse and invalid request errors in %s", out)
	}
}

func TestHugeLineIsRejectedAndServerContinues(t *testing.T) {
	h := startServer(t, setup{})
	h.init()
	huge := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"x":"` + strings.Repeat("a", MaxLineBytes+10) + `"}}`
	h.sendRaw(huge)
	h.sendRaw(`{"jsonrpc":"2.0","id":"ok","method":"ping"}`)
	if m := h.await("ok", 5*time.Second); m["result"] == nil {
		t.Errorf("no ping answer after a huge line")
	}
	h.mu.Lock()
	out := h.raw.String()
	h.mu.Unlock()
	if !strings.Contains(out, "longer than") {
		t.Errorf("the huge line was not reported: %.200s", out)
	}
}

func TestEOFWaitsForInFlightWorkThenExitsCleanly(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	h := startServer(t, setup{groups: [][]mcpkit.Tool{{
		readTool("fake_wait", func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			close(started)
			<-release
			return mcpkit.Text("finished"), nil
		}),
	}}})
	h.init()
	h.request("w", "tools/call", map[string]any{"name": "fake_wait"})
	<-started
	h.in.Close() // EOF on stdin while the tool runs
	select {
	case <-h.served:
		t.Fatal("Serve returned before the in-flight call finished")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-h.served:
		if err != nil {
			t.Errorf("Serve returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after the call finished")
	}
	h.mu.Lock()
	_, ok := h.resp[`"w"`]
	h.mu.Unlock()
	if !ok {
		t.Errorf("the in-flight call was not answered before exit")
	}
}

func TestEOFImmediately(t *testing.T) {
	h := startServer(t, setup{})
	h.in.Close()
	select {
	case err := <-h.served:
		if err != nil {
			t.Errorf("Serve = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return on EOF")
	}
}

func TestContextCancelStopsServe(t *testing.T) {
	reg, _ := NewRegistry(Builtin(NewPlanStore(0, nil)))
	srv, _ := New(Options{Registry: reg, Store: NewPlanStore(0, nil)})
	ctx, cancel := context.WithCancel(context.Background())
	inR, inW := io.Pipe()
	defer inW.Close()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, inR, io.Discard) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve ignored the cancelled context")
	}
}

func TestRedactionNeverReachesStdout(t *testing.T) {
	users, homes := diag.Identity()
	users = append(users, "plantedperson")
	homes = append(homes, `C:\Users\plantedperson`)
	sc := diag.NewScrubber(users, homes)
	secrets := []string{"plantedperson", `C:\Users\plantedperson`}
	for _, u := range users {
		if len(u) >= 4 {
			secrets = append(secrets, u)
		}
	}
	for _, hm := range homes {
		if len(hm) >= 8 {
			secrets = append(secrets, hm)
		}
	}
	dirty := `C:\Users\plantedperson\AppData\Roaming\WinForge\config.json and plantedperson`
	h := startServer(t, setup{redact: sc.Clean, groups: [][]mcpkit.Tool{{
		readTool("fake_leak", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) {
			home, _ := os.UserHomeDir()
			return &mcpkit.Result{
				Text: dirty + " " + home,
				Structured: map[string]any{
					"path": dirty, "home": home, "nested": []any{map[string]any{"p": dirty}},
					"plantedperson": "key leaks too", "n": 12.5,
				},
			}, nil
		}),
		readTool("fake_errleak", func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) {
			return nil, fmt.Errorf("cannot open %s", dirty)
		}),
	}}})
	h.init()
	r := h.tool(1, "fake_leak", nil)
	if !strings.Contains(string(r.Structured), "12.5") || !strings.Contains(r.text(), "%USERPROFILE%") {
		t.Errorf("redaction should keep the shape: %s / %s", r.text(), r.Structured)
	}
	h.tool(2, "fake_errleak", nil)
	h.call(3, "tools/call", map[string]any{"name": "tool_named_plantedperson"})
	h.mu.Lock()
	out := h.raw.String()
	h.mu.Unlock()
	for _, s := range secrets {
		if strings.Contains(strings.ToLower(out), strings.ToLower(s)) {
			t.Errorf("%q reached stdout:\n%s", s, out)
		}
		esc, _ := json.Marshal(s)
		if strings.Contains(strings.ToLower(out), strings.ToLower(strings.Trim(string(esc), `"`))) {
			t.Errorf("JSON-escaped %q reached stdout", s)
		}
	}
}

func TestRegistryRejectsDuplicatesAndBadTools(t *testing.T) {
	ok := func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) { return nil, nil }
	a := mcpkit.Tool{Name: "winforge_a", Handler: ok}
	if _, err := NewRegistry([]mcpkit.Tool{a}, []mcpkit.Tool{a}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate across groups: %v", err)
	}
	if _, err := NewRegistry(Builtin(NewPlanStore(0, nil)), []mcpkit.Tool{{Name: ToolApplyPlan, Handler: ok}}); err == nil {
		t.Errorf("a group must not redefine a built-in tool")
	}
	if _, err := NewRegistry([]mcpkit.Tool{{Name: "", Handler: ok}}); err == nil {
		t.Errorf("nameless tool accepted")
	}
	if _, err := NewRegistry([]mcpkit.Tool{{Name: "x"}}); err == nil {
		t.Errorf("tool without handler accepted")
	}
	if _, err := NewRegistry([]mcpkit.Tool{{Name: "x", Handler: ok, InputSchema: json.RawMessage(`[1]`)}}); err == nil {
		t.Errorf("bad schema accepted")
	}
	r, err := NewRegistry([]mcpkit.Tool{{Name: "winforge_b", Handler: ok}, a})
	if err != nil {
		t.Fatal(err)
	}
	if ts := r.Tools(); ts[0].Name != "winforge_a" || string(ts[0].InputSchema) == "" {
		t.Errorf("tools not sorted or schema not defaulted: %+v", ts)
	}
}

func TestRealRegistryBuilds(t *testing.T) {
	env := RealEnv("test")
	if _, err := RealRegistry(env, NewPlanStore(0, nil)); err != nil {
		t.Fatal(err)
	}
	if env.Redact == nil || env.Catalog == nil || env.LockPath == nil || env.AppData == nil {
		t.Errorf("RealEnv left a field empty: %+v", env)
	}
	cat, err := env.Catalog()
	if err != nil || len(cat.Apps) == 0 {
		t.Errorf("catalog: %v", err)
	}
	if again, _ := env.Catalog(); again != cat {
		t.Errorf("catalog should be cached")
	}
}

// ---- plan store ----

func TestPlanStoreLimitsAndConcurrency(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	s := NewPlanStore(time.Minute, clock.Now)
	var ids []string
	for i := 0; i < MaxPlans+5; i++ {
		id, _ := s.Put(mcpkit.Plan{Kind: fmt.Sprint(i)})
		ids = append(ids, id)
	}
	if n := len(s.List()); n != MaxPlans {
		t.Errorf("kept %d plans, want %d", n, MaxPlans)
	}
	if _, err := s.Take(ids[0]); err == nil {
		t.Errorf("the oldest plan should have been dropped")
	}
	if _, err := s.Take(ids[len(ids)-1]); err != nil {
		t.Errorf("the newest plan is missing: %v", err)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] || len(id) != 32 {
			t.Fatalf("bad or repeated id %q", id)
		}
		seen[id] = true
	}

	// Many goroutines taking the same plan: exactly one wins.
	id, _ := s.Put(mcpkit.Plan{Kind: "race"})
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Take(id); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Errorf("%d goroutines got the same plan", wins.Load())
	}
	// Concurrent Put/List/Discard must not corrupt the store.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, _ := s.Put(mcpkit.Plan{})
			s.List()
			s.Discard(id)
		}()
	}
	wg.Wait()
	clock.Add(2 * time.Minute)
	if n := len(s.List()); n != 0 {
		t.Errorf("%d plans survived their TTL", n)
	}
}
