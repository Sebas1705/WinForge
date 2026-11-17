// Package actions holds the MCP tools that change the PC, in two steps: a winforge_prepare_* tool re-reads
// the machine, validates what was asked against what it lists right now, stores a plan with
// mcpkit.Call.Plans.Put and changes nothing; the server's generic winforge_apply_plan then runs the stored
// plan once, and only in full mode. See internal/mcpkit for the contract.
//
// The tools mirror the GUI bindings (storagebind.go, privacybind.go, envbind.go, startupbind.go,
// servicesbind.go, tasksbind.go, featuresbind.go) but not their session state: the allow-list a binding
// keeps ("only what a scan listed") is here a fresh scan inside prepare and a second check inside Run.
// Nothing accepts a free-form command, script or path: ids come from fixed lists, and the only paths
// accepted are ones a fresh project search lists.
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// Tools returns this group's tools on the real machine.
func Tools(env *mcpkit.Env) []mcpkit.Tool { return ToolsWith(env, RealDeps(env)) }

// ToolsWith returns this group's tools over the given machine; tests pass fakes.
func ToolsWith(env *mcpkit.Env, d Deps) []mcpkit.Tool {
	k := &kit{env: env, d: d}
	return []mcpkit.Tool{
		k.storageCleanTool(),
		k.projectJunkTool(),
		k.privacyTool(),
		k.privacyRestoreTool(),
		k.envFixTool(),
		k.envRestoreTool(),
		k.startupToggleTool(),
		k.serviceModeTool(),
		k.serviceRestoreTool(),
		k.taskToggleTool(),
		k.taskRestoreTool(),
		k.featureTool(),
		k.optimizeTool(),
	}
}

// prepNote ends every prepare tool's description.
const prepNote = " This tool changes nothing: it only builds a plan and stores it. Nothing happens until " +
	"winforge_apply_plan is called with the returned planId (full mode only; the plan runs once and expires). " +
	"Before that, show the person the summary and get their explicit approval."

type kit struct {
	env *mcpkit.Env
	d   Deps
}

// opMu makes one change at a time across every tool of this group, like the GUI's per-page op locks.
var opMu sync.Mutex

type runFunc func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error)

func (k *kit) redact(s string) string {
	if k.env != nil && k.env.Redact != nil {
		return k.env.Redact(s)
	}
	return s
}

func (k *kit) redactAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = k.redact(s)
	}
	return out
}

func (k *kit) elevated() bool { return k.env != nil && k.env.Elevated != nil && k.env.Elevated() }

func (k *kit) now() time.Time {
	if k.env != nil && k.env.Now != nil {
		return k.env.Now()
	}
	return time.Now()
}

// refuse is a plain answer for a request that cannot be planned; nothing has been stored.
func refuse(format string, a ...any) (*mcpkit.Result, error) { return mcpkit.Fail(format, a...), nil }

// notAvailable answers when a backend was not given.
func notAvailable(what string) (*mcpkit.Result, error) {
	return refuse("%s is not available in this build", what)
}

// prepared is what every prepare tool returns.
type prepared struct {
	PlanID     string `json:"planId"`
	Kind       string `json:"kind"`
	Summary    any    `json:"summary"`
	NeedsAdmin bool   `json:"needsAdmin"`
	ExpiresAt  string `json:"expiresAt"`
}

// store keeps the plan and answers with its id. It is always the last step of a prepare handler, so
// nothing is stored when anything before it failed.
func (k *kit) store(c *mcpkit.Call, kind string, summary any, admin bool, run runFunc) (*mcpkit.Result, error) {
	if c.Plans == nil {
		return refuse("there is no plan store, so nothing can be prepared")
	}
	id, exp := c.Plans.Put(mcpkit.Plan{Kind: kind, Summary: summary, Admin: admin, Run: k.guard(run)})
	return mcpkit.JSON(prepared{PlanID: id, Kind: kind, Summary: summary, NeedsAdmin: admin, ExpiresAt: exp.UTC().Format(time.RFC3339)}), nil
}

// guard makes a plan's Run safe to call: at most once whatever the store does, one change at a time, and
// never after the request was cancelled.
func (k *kit) guard(run runFunc) runFunc {
	var used sync.Once
	return func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		first := false
		used.Do(func() { first = true })
		if !first {
			return refuse("this plan was already run; prepare a new one if the person wants it again")
		}
		opMu.Lock()
		defer opMu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return run(ctx, c)
	}
}

// decode reads strict arguments; a bad request is answered, not raised.
func decode(c *mcpkit.Call, v any) (*mcpkit.Result, bool) {
	if err := mcpkit.Decode(c.Args, v); err != nil {
		return mcpkit.Fail("%s", err.Error()), false
	}
	return nil, true
}

// progress is c.Progress, safe when the client did not ask for it.
func progress(c *mcpkit.Call, done, total float64, msg string) {
	if c != nil && c.Progress != nil {
		c.Progress(done, total, msg)
	}
}

// outcome is a Result for a change: structured data, an error when nothing worked.
func outcome(v any, failed bool) *mcpkit.Result {
	r := mcpkit.JSON(v)
	r.IsError = failed
	return r
}

// human formats a byte count.
func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func identity(s string) string { return s }

// match finds the one key that equals want (after norm), or whose redacted form does. A model reads
// ids and paths through the server's redaction, so what it sends back may be the redacted form of
// the real one; the real one is what the plan keeps. ambiguous says several redacted forms matched.
func (k *kit) match(want string, keys []string, norm func(string) string) (at int, ok, ambiguous bool) {
	for i, key := range keys {
		if norm(key) == norm(want) {
			return i, true, false
		}
	}
	n := 0
	for i, key := range keys {
		if norm(k.redact(key)) == norm(want) {
			n++
			at = i
		}
	}
	if n == 1 {
		return at, true, false
	}
	return -1, false, n > 1
}

// schemaJSON builds a strict object schema from a Go value (used where an enum is generated).
func schemaJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("actions: schema: " + err.Error())
	}
	return mcpkit.Schema(string(b))
}
