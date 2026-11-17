// Package pcread holds the read-only MCP tools that look at this PC: health, storage, startup, developer
// environment, Windows features and privacy settings. Every tool only reads (see the Deps backends); none of
// them changes a file, a setting or the registry. What a tool returns is seen by the model, so each tool says
// in its description whether it describes this PC's files, folders or settings, and every path or text that
// comes from the PC passes through Env.Redact before it leaves.
package pcread

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// Tools returns the group's tools backed by the real machine.
func Tools(env *mcpkit.Env) []mcpkit.Tool { return ToolsWith(env, RealDeps()) }

// ToolsWith is Tools with the backends replaced, for tests. A nil func field in deps is filled with the real
// backend, so a test only fakes what it uses.
func ToolsWith(env *mcpkit.Env, deps Deps) []mcpkit.Tool {
	deps = deps.withDefaults()
	g := &group{env: env, d: deps}
	var out []mcpkit.Tool
	out = append(out, g.healthTools()...)
	out = append(out, g.storageTools()...)
	out = append(out, g.startupTools()...)
	out = append(out, g.devTools()...)
	out = append(out, g.systemTools()...)
	return out
}

// group carries what every handler needs.
type group struct {
	env *mcpkit.Env
	d   Deps
}

// tc is the per-call context handlers use.
type tc struct {
	c      *mcpkit.Call
	env    *mcpkit.Env
	now    func() time.Time
	redact func(string) string
}

func (x *tc) progress(done, total float64, msg string) {
	if x.c.Progress != nil {
		x.c.Progress(done, total, x.redact(msg))
	}
}

func (x *tc) elevated() bool { return x.env != nil && x.env.Elevated != nil && x.env.Elevated() }

// read builds a Read-class tool whose handler cannot panic out: a panic becomes a tool error, a cancelled ctx
// becomes a "cancelled" answer, and a backend error becomes a redacted tool error.
func (g *group) read(name, title, desc string, schema []byte, h func(ctx context.Context, x *tc) (*mcpkit.Result, error)) mcpkit.Tool {
	return mcpkit.Tool{
		Name: name, Title: title, Description: desc, InputSchema: schema, Class: mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (res *mcpkit.Result, err error) {
			defer func() {
				if r := recover(); r != nil {
					res, err = mcpkit.Fail("%s failed unexpectedly; nothing was changed", name), nil
				}
			}()
			if c == nil {
				c = &mcpkit.Call{}
			}
			env := c.Env
			if env == nil {
				env = g.env
			}
			x := &tc{c: c, env: env, now: time.Now, redact: redactor(env)}
			if env != nil && env.Now != nil {
				x.now = env.Now
			}
			if ctx.Err() != nil {
				return mcpkit.Fail("%s was cancelled before it started", name), nil
			}
			res, err = h(ctx, x)
			if err != nil {
				if ctx.Err() != nil {
					return mcpkit.Fail("%s was cancelled", name), nil
				}
				return mcpkit.Fail("%s failed: %s", name, x.redact(err.Error())), nil
			}
			return res, nil
		},
	}
}

// redactor returns env.Redact, or a conservative fallback that removes the user name and home folder when the
// environment has none, so a missing Redact never means "no redaction".
func redactor(env *mcpkit.Env) func(string) string {
	if env != nil && env.Redact != nil {
		return env.Redact
	}
	home, _ := os.UserHomeDir()
	user := os.Getenv("USERNAME")
	if user == "" {
		user = os.Getenv("USER")
	}
	return func(s string) string {
		if home != "" {
			s = replaceFold(s, home, "~")
			s = replaceFold(s, strings.ReplaceAll(home, `\`, "/"), "~")
		}
		if len(user) >= 3 {
			s = replaceFold(s, user, "<user>")
		}
		return s
	}
}

func replaceFold(s, old, repl string) string {
	if old == "" {
		return s
	}
	ls, lo := strings.ToLower(s), strings.ToLower(old)
	var b strings.Builder
	for {
		i := strings.Index(ls, lo)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString(repl)
		s, ls = s[i+len(old):], ls[i+len(old):]
	}
}

// onWindows decides which absolute-path shape checkFolder accepts besides a drive letter.
var onWindows = runtime.GOOS == "windows"
