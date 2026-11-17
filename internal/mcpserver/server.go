package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/sched"
)

// Limits and defaults.
const (
	MaxLineBytes       = 4 << 20 // longest accepted JSON-RPC line
	DefaultConcurrency = 4
	DefaultReadTimeout = 5 * time.Minute
	DefaultActTimeout  = 45 * time.Minute
	maxPending         = 256
	shutdownGrace      = 15 * time.Second
)

// Protocol versions this server speaks, newest first.
var supportedVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// Instructions is what the model is told when it connects.
const Instructions = `WinForge on this Windows PC. You are reading and, only when the person allowed it, changing their computer, so be careful.

- Everything a tool returns is sent to you, the model, and so leaves the PC. Ask for the least data that answers the question and do not repeat it back more than needed.
- Prefer read tools. They only look. A tool that is not listed is not allowed in this session's mode.
- A change always goes in two steps: a prepare tool returns a plan with a plain summary and a plan id; show that summary to the person and wait for their clear approval; only then call winforge_apply_plan with that id. Never apply a plan the person has not approved in this conversation, never reuse or guess an id, and use winforge_discard_plan when they decline.
- Plan ids work once and expire after a few minutes. If one expired, prepare the plan again and ask again.
- If a tool says it needs administrator rights or that another run is busy, tell the person; do not look for a way around it.`

// Options configures a Server.
type Options struct {
	Mode          Mode
	Registry      *Registry
	Store         *PlanStore
	Env           *mcpkit.Env
	Version       string
	ReadTimeout   time.Duration // default DefaultReadTimeout; applies to Read and Prepare tools
	ActTimeout    time.Duration // default DefaultActTimeout
	MaxConcurrent int           // default DefaultConcurrency
	Log           io.Writer     // stderr in the real server; nil discards
}

// Server speaks MCP over one reader and one writer.
type Server struct {
	opt   Options
	sem   chan struct{} // concurrency cap
	actMu chan struct{} // one Act tool at a time

	outMu sync.Mutex
	w     io.Writer

	logMu sync.Mutex

	mu       sync.Mutex
	inflight map[string]context.CancelFunc

	pending     atomic.Int64
	initialized atomic.Bool
	handlers    sync.WaitGroup // tool handler goroutines (they may outlive a timed-out request)
}

// New builds a server. Registry and Store are required.
func New(o Options) (*Server, error) {
	if o.Registry == nil || o.Store == nil {
		return nil, errors.New("mcpserver: a registry and a plan store are required")
	}
	if o.Env == nil {
		o.Env = &mcpkit.Env{}
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = DefaultReadTimeout
	}
	if o.ActTimeout <= 0 {
		o.ActTimeout = DefaultActTimeout
	}
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = DefaultConcurrency
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	return &Server{
		opt:      o,
		sem:      make(chan struct{}, o.MaxConcurrent),
		actMu:    make(chan struct{}, 1),
		inflight: map[string]context.CancelFunc{},
	}, nil
}

// ---- wire types ----

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// ---- output ----

func (s *Server) redact(text string) string {
	if s.opt.Env != nil && s.opt.Env.Redact != nil {
		return s.opt.Env.Redact(text)
	}
	return text
}

func (s *Server) logf(format string, a ...any) {
	if s.opt.Log == nil {
		return
	}
	line := s.redact(fmt.Sprintf("winforge-mcp: "+format, a...))
	s.logMu.Lock()
	defer s.logMu.Unlock()
	fmt.Fprintln(s.opt.Log, strings.TrimRight(line, "\r\n"))
}

// write sends one JSON message as one line. stdout carries nothing else.
func (s *Server) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		s.logf("could not encode a message: %v", err)
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if _, err := s.w.Write(append(b, '\n')); err != nil {
		s.logf("could not write to the client: %v", err)
	}
}

func (s *Server) respond(id json.RawMessage, result any) {
	b, err := json.Marshal(result)
	if err != nil {
		s.respondErr(id, codeInternal, "could not encode the result")
		return
	}
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: b})
}

func (s *Server) respondErr(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: s.redact(msg)}})
}

// ---- input loop ----

type inLine struct {
	data    []byte
	tooLong bool
	err     error
}

// readLines splits r into lines, never holding more than MaxLineBytes of one line: a longer line is skipped
// to its end and reported as tooLong.
func readLines(ctx context.Context, r io.Reader, out chan<- inLine) {
	defer close(out)
	br := bufio.NewReaderSize(r, 64<<10)
	send := func(l inLine) bool {
		select {
		case out <- l:
			return true
		case <-ctx.Done():
			return false
		}
	}
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := br.ReadLine()
		if len(chunk) > 0 && !tooLong {
			if len(buf)+len(chunk) > MaxLineBytes {
				tooLong = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		if err != nil {
			if len(buf) > 0 && !tooLong {
				send(inLine{data: buf})
			} else if tooLong {
				send(inLine{tooLong: true})
			}
			if err != io.EOF {
				send(inLine{err: err})
			}
			return
		}
		if isPrefix {
			continue
		}
		var ok bool
		if tooLong {
			ok = send(inLine{tooLong: true})
		} else {
			ok = send(inLine{data: buf})
		}
		if !ok {
			return
		}
		buf, tooLong = nil, false
	}
}

// Serve reads requests from r and writes responses to w until r ends or ctx is cancelled. On end of input it
// waits for the requests in flight and returns nil.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	s.w = w
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan inLine)
	go readLines(ctx, r, lines)

	var wg sync.WaitGroup // request dispatchers
	var rerr error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case l, ok := <-lines:
			if !ok {
				break loop
			}
			if l.err != nil {
				rerr = l.err
				break loop
			}
			if l.tooLong {
				s.respondErr(nil, codeInvalidRequest, fmt.Sprintf("message longer than %d bytes was ignored", MaxLineBytes))
				continue
			}
			s.handleLine(ctx, l.data, &wg)
		}
	}
	if ctx.Err() != nil { // shut down: stop the work in flight
		cancel()
	}
	wg.Wait()
	done := make(chan struct{})
	go func() { s.handlers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		s.logf("a tool did not stop within %s of shutting down", shutdownGrace)
	}
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return rerr
	}
	return nil
}

func (s *Server) handleLine(ctx context.Context, data []byte, wg *sync.WaitGroup) {
	data = bytes.TrimPrefix(bytes.TrimSpace(data), []byte("\xef\xbb\xbf"))
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return
	}
	if data[0] == '[' {
		s.respondErr(nil, codeInvalidRequest, "batches are not supported: send one message per line")
		return
	}
	var m rpcMessage
	if err := json.Unmarshal(data, &m); err != nil {
		s.respondErr(nil, codeParse, "parse error: the line is not valid JSON")
		return
	}
	hasID := len(m.ID) > 0
	switch {
	case m.Method == "" && (len(m.Result) > 0 || len(m.Error) > 0):
		return // a response to something we never sent
	case m.JSONRPC != "2.0" || m.Method == "":
		s.respondErr(m.ID, codeInvalidRequest, "invalid request: expected a JSON-RPC 2.0 message with a method")
		return
	case !hasID:
		s.handleNotification(m)
		return
	}
	if string(m.ID) == "null" || (m.ID[0] != '"' && !isNumber(m.ID)) {
		s.respondErr(nil, codeInvalidRequest, "invalid request: the id must be a string or a number")
		return
	}
	switch m.Method {
	case "initialize":
		s.handleInitialize(m)
		return
	case "ping":
		s.respond(m.ID, struct{}{})
		return
	}
	if m.Method != "tools/list" && m.Method != "tools/call" {
		s.respondErr(m.ID, codeMethodNotFound, "method not found: "+m.Method)
		return
	}
	if !s.initialized.Load() {
		s.respondErr(m.ID, codeInvalidRequest, "the server is not initialized: send initialize first")
		return
	}
	if s.pending.Add(1) > maxPending {
		s.pending.Add(-1)
		s.respondErr(m.ID, codeInternal, "too many requests in flight")
		return
	}
	key := string(m.ID)
	rctx, rcancel := context.WithCancel(ctx)
	s.mu.Lock()
	if _, dup := s.inflight[key]; dup {
		s.mu.Unlock()
		rcancel()
		s.pending.Add(-1)
		s.respondErr(m.ID, codeInvalidRequest, "a request with this id is already in flight")
		return
	}
	s.inflight[key] = rcancel
	s.mu.Unlock()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer s.pending.Add(-1)
		defer func() {
			s.mu.Lock()
			delete(s.inflight, key)
			s.mu.Unlock()
			rcancel()
		}()
		select { // the concurrency cap
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-rctx.Done():
			return
		}
		s.dispatch(rctx, m)
	}()
}

func isNumber(b json.RawMessage) bool {
	var n json.Number
	return json.Unmarshal(b, &n) == nil && len(b) > 0 && b[0] != '"'
}

func (s *Server) handleNotification(m rpcMessage) {
	switch m.Method {
	case "notifications/initialized":
		s.initialized.Store(true)
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
			Reason    string          `json:"reason"`
		}
		if json.Unmarshal(m.Params, &p) != nil || len(p.RequestID) == 0 {
			return
		}
		s.mu.Lock()
		cancel := s.inflight[string(p.RequestID)]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
	// Anything else is ignored, as the protocol asks for unknown notifications.
}

func (s *Server) handleInitialize(m rpcMessage) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	if len(m.Params) > 0 {
		if err := json.Unmarshal(m.Params, &p); err != nil {
			s.respondErr(m.ID, codeInvalidParams, "invalid params for initialize")
			return
		}
	}
	version := supportedVersions[0]
	for _, v := range supportedVersions {
		if p.ProtocolVersion == v {
			version = v
		}
	}
	s.initialized.Store(true) // tools/list may follow before the notification arrives
	s.logf("client %s %s connected, protocol %s, mode %s", p.ClientInfo.Name, p.ClientInfo.Version, version, s.opt.Mode)
	s.respond(m.ID, map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "winforge", "version": s.opt.Version},
		"instructions":    Instructions,
	})
}

// ---- requests ----

func (s *Server) dispatch(ctx context.Context, m rpcMessage) {
	switch m.Method {
	case "tools/list":
		s.respond(m.ID, map[string]any{"tools": s.opt.Registry.ToolList(s.opt.Mode)})
	case "tools/call":
		s.toolsCall(ctx, m)
	}
}

type outcome struct {
	res *mcpkit.Result
	err error
}

func (s *Server) toolsCall(ctx context.Context, m rpcMessage) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Meta      struct {
			ProgressToken json.RawMessage `json:"progressToken"`
		} `json:"_meta"`
	}
	if len(m.Params) == 0 || json.Unmarshal(m.Params, &p) != nil || p.Name == "" {
		s.respondErr(m.ID, codeInvalidParams, "invalid params: tools/call needs a tool name")
		return
	}
	if a := bytes.TrimSpace(p.Arguments); len(a) > 0 && string(a) != "null" && a[0] != '{' {
		s.respondErr(m.ID, codeInvalidParams, "invalid params: arguments must be an object")
		return
	}
	tool, ok := s.opt.Registry.Tool(p.Name)
	if !ok {
		s.respondErr(m.ID, codeInvalidParams, "unknown tool: "+p.Name)
		return
	}
	if !s.opt.Mode.Allows(tool.Class) {
		s.sendResult(m.ID, mcpkit.Fail("The tool %s is disabled: this server runs in --mode %s and the tool needs --mode %s. "+
			"The person can enable it by restarting the MCP server with that flag (for example: claude mcp add winforge -- <path>\\winforge-mcp.exe --mode %s). "+
			"Do not try to work around this.", tool.Name, s.opt.Mode, modeFor(tool.Class), modeFor(tool.Class)))
		return
	}

	timeout := s.opt.ReadTimeout
	if tool.Class == mcpkit.Act {
		timeout = s.opt.ActTimeout
	}
	tctx, tcancel := context.WithTimeout(ctx, timeout)
	defer tcancel()

	var progMu sync.Mutex
	progClosed := false
	last := -1.0
	token := p.Meta.ProgressToken
	progress := func(done, total float64, message string) {
		if len(token) == 0 {
			return
		}
		progMu.Lock()
		defer progMu.Unlock()
		if progClosed || done <= last {
			return
		}
		last = done
		params := map[string]any{"progressToken": token, "progress": done}
		if total > 0 {
			params["total"] = total
		}
		if message != "" {
			params["message"] = s.redact(message)
		}
		s.write(rpcNotification{JSONRPC: "2.0", Method: "notifications/progress", Params: params})
	}
	defer func() {
		progMu.Lock()
		progClosed = true
		progMu.Unlock()
	}()

	call := &mcpkit.Call{Args: p.Arguments, Env: s.opt.Env, Plans: s.opt.Store, Progress: progress}
	done := make(chan outcome, 1)
	s.handlers.Add(1)
	go func() {
		defer s.handlers.Done()
		var out outcome
		defer func() { done <- out }()
		defer func() {
			if r := recover(); r != nil {
				s.logf("panic in tool %s: %v\n%s", tool.Name, r, debug.Stack())
				out = outcome{err: fmt.Errorf("the tool %s failed unexpectedly (an internal error was logged on the server)", tool.Name)}
			}
		}()
		if tool.Class == mcpkit.Act {
			release, busy := s.enterAct(tctx)
			if busy != nil {
				out.res = busy
				return
			}
			defer release()
		}
		out.res, out.err = tool.Handler(tctx, call)
	}()

	var out outcome
	select {
	case out = <-done:
	case <-tctx.Done():
		select { // a result that arrived at the same moment wins
		case out = <-done:
		default:
			if errors.Is(tctx.Err(), context.DeadlineExceeded) {
				s.sendResult(m.ID, mcpkit.Fail("The tool %s did not finish within %s and was stopped.", tool.Name, timeout))
			}
			// Cancelled by the client or by shutdown: the client no longer waits for an answer.
			return
		}
	}
	if ctx.Err() != nil {
		return // the client cancelled (or the server is shutting down): nobody waits for the answer
	}
	if errors.Is(tctx.Err(), context.DeadlineExceeded) && out.err != nil {
		s.sendResult(m.ID, mcpkit.Fail("The tool %s did not finish within %s and was stopped.", tool.Name, timeout))
		return
	}
	if out.err != nil {
		s.sendResult(m.ID, mcpkit.Fail("%s", out.err.Error()))
		return
	}
	if out.res == nil {
		out.res = mcpkit.Text("Done.")
	}
	s.sendResult(m.ID, out.res)
}

// enterAct makes Act tools run one at a time, and never while `winforge-cli apply`, `updates` or the app
// hold the shared run lock. The returned result is the tool error when it could not start.
func (s *Server) enterAct(ctx context.Context) (release func(), busy *mcpkit.Result) {
	select {
	case s.actMu <- struct{}{}:
	case <-ctx.Done():
		return nil, mcpkit.Fail("Another change is still running and this one gave up waiting. Nothing was changed.")
	}
	unlock := func() {}
	if env := s.opt.Env; env != nil && env.LockPath != nil {
		path, err := env.LockPath()
		if err == nil {
			unlock, err = sched.Lock{Path: path, Now: env.Now}.Acquire()
		}
		if err != nil {
			<-s.actMu
			if errors.Is(err, sched.ErrBusy) {
				return nil, mcpkit.Fail("Another WinForge run (apply, updates or the app) is in progress, so nothing was changed. Try again when it has finished.")
			}
			return nil, mcpkit.Fail("Could not take the WinForge run lock, so nothing was changed: %v", err)
		}
	}
	return func() { unlock(); <-s.actMu }, nil
}

// sendResult redacts a tool result and sends it.
func (s *Server) sendResult(id json.RawMessage, r *mcpkit.Result) {
	out := map[string]any{
		"content": []map[string]any{{"type": "text", "text": s.redact(r.Text)}},
		"isError": r.IsError,
	}
	if r.Structured != nil {
		if v, ok := s.redactStructured(r.Structured); ok {
			out["structuredContent"] = v
		} else {
			out["content"] = []map[string]any{{"type": "text", "text": s.redact(r.Text) + "\n(structured data could not be encoded)"}}
		}
	}
	s.respond(id, out)
}

// redactStructured round-trips v through JSON and redacts every string and every object key. The MCP schema
// wants an object, so other shapes are wrapped as {"result": ...}.
func (s *Server) redactStructured(v any) (any, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var g any
	if dec.Decode(&g) != nil {
		return nil, false
	}
	g = redactAny(g, s.redact)
	if _, isObj := g.(map[string]any); !isObj {
		g = map[string]any{"result": g}
	}
	return g, true
}

func redactAny(v any, f func(string) string) any {
	switch t := v.(type) {
	case string:
		return f(t)
	case []any:
		for i := range t {
			t[i] = redactAny(t[i], f)
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[f(k)] = redactAny(e, f)
		}
		return out
	}
	return v
}
