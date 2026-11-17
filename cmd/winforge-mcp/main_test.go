package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/mcpserver"
)

func fakeEnv(string) *mcpkit.Env { return &mcpkit.Env{} }

func runArgs(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(context.Background(), args, strings.NewReader(stdin), &out, &errb, fakeEnv)
	return code, out.String(), errb.String()
}

func TestListToolsPerMode(t *testing.T) {
	names := map[string][]string{}
	for _, mode := range []string{"read", "plan", "full"} {
		code, out, _ := runArgs(t, "", "--list-tools", "--mode", mode)
		if code != exitcode.OK {
			t.Fatalf("%s: exit %d", mode, code)
		}
		var doc struct {
			Mode  string
			Tools []struct{ Name string }
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Mode != mode {
			t.Fatalf("%s: bad document %q: %v", mode, out, err)
		}
		for _, tl := range doc.Tools {
			names[mode] = append(names[mode], tl.Name)
		}
	}
	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	if !has(names["read"], mcpserver.ToolListPlans) || has(names["read"], mcpserver.ToolApplyPlan) || has(names["read"], mcpserver.ToolDiscardPlan) {
		t.Errorf("read tools: %v", names["read"])
	}
	if !has(names["plan"], mcpserver.ToolDiscardPlan) || has(names["plan"], mcpserver.ToolApplyPlan) {
		t.Errorf("plan tools: %v", names["plan"])
	}
	if !has(names["full"], mcpserver.ToolApplyPlan) {
		t.Errorf("full tools: %v", names["full"])
	}
	// The default is read.
	_, out, _ := runArgs(t, "", "--list-tools")
	if strings.Contains(out, mcpserver.ToolApplyPlan) {
		t.Errorf("the default mode offers the apply tool")
	}
}

func TestFlagsAndExitCodes(t *testing.T) {
	if code, out, _ := runArgs(t, "", "--version"); code != 0 || !strings.HasPrefix(out, "winforge-mcp ") {
		t.Errorf("--version: %d %q", code, out)
	}
	code, out, _ := runArgs(t, "", "--help")
	if code != 0 || !strings.Contains(out, "claude mcp add winforge") || !strings.Contains(out, "--mode full") || !strings.Contains(out, "PRIVACY") {
		t.Errorf("--help: %d %q", code, out)
	}
	for _, args := range [][]string{{"--mode", "root"}, {"--bogus"}, {"extra"}, {"--plan-ttl", "0s"}, {"--read-timeout", "-1m"}, {"--act-timeout", "abc"}} {
		if code, _, errs := runArgs(t, "", args...); code != exitcode.Usage || errs == "" {
			t.Errorf("%v: exit %d, stderr %q", args, code, errs)
		}
	}
}

func TestServesOverStdinAndExitsOnEOF(t *testing.T) {
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"winforge_apply_plan","arguments":{"planId":"x"}}}
`
	code, out, errs := runArgs(t, in)
	if code != exitcode.OK {
		t.Fatalf("exit %d, stderr %s", code, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 responses, got %d: %q", len(lines), out)
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Errorf("stdout line is not JSON: %q", l)
		}
	}
	if !strings.Contains(out, "--mode full") {
		t.Errorf("apply must be refused in the default mode: %s", out)
	}
	if strings.Contains(out, "winforge-mcp ready") {
		t.Errorf("a log line reached stdout")
	}
}
