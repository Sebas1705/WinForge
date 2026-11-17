// Package mcpkit is the small contract between the MCP server (internal/mcpserver) and the groups of tools
// (internal/mcptools/...). A tool is data plus a handler; the server owns the protocol, the safety mode, the
// plan store, progress, cancellation and redaction, so a tool only has to do its job and return a Result.
package mcpkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
)

// Class says what a tool may do. The server refuses a tool whose class the current mode does not allow.
type Class int

const (
	// Read tools only look: they change nothing on the PC and write nothing but their own caches.
	Read Class = iota
	// Prepare tools build a plan and store it with Call.Plans.Put. They change nothing; the result carries the
	// plan id, a plain summary of exactly what would happen, and when the plan expires.
	Prepare
	// Act tools run something that changes the PC. The only generic one is winforge_apply_plan, which runs a
	// plan a Prepare tool stored. A tool of this class never accepts free-form targets: it takes a plan id.
	Act
)

// String is the name used in docs and in the tool annotations.
func (c Class) String() string {
	switch c {
	case Read:
		return "read"
	case Prepare:
		return "prepare"
	case Act:
		return "act"
	}
	return "unknown"
}

// Tool is one MCP tool.
type Tool struct {
	Name        string // snake_case, prefixed winforge_
	Title       string // short, for people
	Description string // what it does, what it returns, and what it never does; the model reads this
	InputSchema json.RawMessage
	Class       Class
	// Handler must return quickly for Read and Prepare tools or report Progress, and must stop when ctx is
	// cancelled. A returned error is shown as a tool error with its text (already redacted by the server).
	Handler func(ctx context.Context, c *Call) (*Result, error)
}

// Call is what a handler gets.
type Call struct {
	Args  json.RawMessage
	Env   *Env
	Plans PlanStore
	// Progress reports work done so far; total may be 0 when unknown. It is a no-op when the client did not ask.
	Progress func(done, total float64, message string)
}

// Env is everything a tool may need from the machine. Tests fill it with fakes; Real() fills it for use.
type Env struct {
	Version  string
	Catalog  func() (*catalog.Catalog, error)
	Snapshot func(context.Context) (system.Inventory, error)
	Exec     install.Executor
	Elevated func() bool
	// LockPath is the lock shared with `winforge-cli apply` and `updates`: two winget runs must not overlap.
	LockPath func() (string, error)
	Now      func() time.Time
	// AppData is WinForge's own folder (%AppData%\WinForge).
	AppData func() (string, error)
	// Redact removes the user name and the user folder from any text. Tools use it on paths they return; the
	// server applies it again to every result as a last gate.
	Redact func(string) string
}

// Plan is something a Prepare tool stored and winforge_apply_plan may run once.
type Plan struct {
	Kind    string // e.g. "install", "storage-clean", "privacy"
	Summary any    // exactly what would happen, as data; it is what the person approved
	Admin   bool   // needs administrator rights
	// Run executes it. It is called at most once, only in a mode that allows Act, and must re-check what it can
	// (the plan is a promise about the state when it was made).
	Run func(ctx context.Context, c *Call) (*Result, error)
}

// PlanStore holds plans between prepare and apply. Ids are random, single use and expire.
type PlanStore interface {
	// Put stores a plan and returns its id and when it expires.
	Put(p Plan) (id string, expires time.Time)
	// Take returns a plan once. ErrPlanUnknown covers never existed, already used and expired.
	Take(id string) (Plan, error)
}

// ErrPlanUnknown is returned by Take for an id that is not (or no longer) valid.
var ErrPlanUnknown = errors.New("plan not found, already used or expired")

// Result is a tool's answer: a short text for the model and, when there is data, the same data as JSON.
type Result struct {
	Text       string
	Structured any
	IsError    bool
}

// Text is a plain answer.
func Text(format string, a ...any) *Result { return &Result{Text: fmt.Sprintf(format, a...)} }

// JSON is a data answer; the text form is the indented JSON so clients that ignore structured content still work.
func JSON(v any) *Result {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return &Result{Text: "could not encode the result: " + err.Error(), IsError: true}
	}
	return &Result{Text: string(b), Structured: v}
}

// Fail is an error answer the model can read and act on.
func Fail(format string, a ...any) *Result {
	return &Result{Text: fmt.Sprintf(format, a...), IsError: true}
}

// Decode reads tool arguments strictly: unknown fields and trailing data are errors, and an empty argument
// object is fine for tools that take none.
func Decode(args json.RawMessage, v any) error {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	if dec.More() {
		return errors.New("invalid arguments: trailing data")
	}
	return nil
}

// Schema checks that s is a JSON object and returns it; it panics at start-up on a typo, never at run time.
func Schema(s string) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil || m["type"] != "object" {
		panic("mcpkit.Schema: not a JSON object schema: " + s)
	}
	return json.RawMessage(s)
}

// NoArgs is the schema of a tool without arguments.
var NoArgs = Schema(`{"type":"object","properties":{},"additionalProperties":false}`)
