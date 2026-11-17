// Command winforge-mcp is a local MCP (Model Context Protocol) server over stdio: an assistant such as Claude
// can ask the WinForge installation on this PC about it and, only when the person allowed it, have it act.
// It opens no port and makes no network call; see --help for the safety modes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/mcpserver"
)

// version is stamped at release time with -ldflags "-X main.version=...".
var version = "dev"

const help = `winforge-mcp: a local MCP server so an assistant can ask WinForge about this PC.

usage:
  winforge-mcp [--mode read|plan|full] [--plan-ttl <time>] [--read-timeout <time>] [--act-timeout <time>]
  winforge-mcp --list-tools [--mode read|plan|full]
  winforge-mcp --version

It speaks MCP (JSON-RPC 2.0, one message per line) on stdin and stdout, opens no port and makes no
network call. Logs go to stderr.

modes (what the assistant may do):
  read   (default) only tools that look at this PC.
  plan   also tools that prepare a plan. Nothing can be applied.
  full   also winforge_apply_plan, which runs a plan prepared in this session, once, after the
         assistant has shown it to you and you approved it. It never runs anything else, and it
         takes the same run lock as "winforge-cli apply" and "updates", so runs never overlap.
A tool the mode does not allow is not listed and a direct call is refused.

register it with Claude Code:
  claude mcp add winforge -- <path>\winforge-mcp.exe
  claude mcp add winforge -- <path>\winforge-mcp.exe --mode full

PRIVACY: whatever a tool returns is sent to the assistant, which is a model that may run in the
cloud. Your user name and your user folder are replaced before anything leaves this program, but
the list of apps, drivers, folders and settings the tools return is shown to the model.

options:
  --mode <m>             read, plan or full (default read)
  --plan-ttl <time>      how long a prepared plan can be applied (default 10m)
  --read-timeout <time>  limit of a read or prepare call (default 5m)
  --act-timeout <time>   limit of an action (default 45m)
  --list-tools           print the tools the chosen mode offers, as JSON, and exit
  --version              print the version and exit

exit codes (shared with winforge-cli):
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, mcpserver.RealEnv)
	stop()
	os.Exit(code)
}

// run is main with its inputs injected so tests can drive it. newEnv builds the machine access.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, newEnv func(version string) *mcpkit.Env) int {
	fs := flag.NewFlagSet("winforge-mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	modeS := fs.String("mode", "read", "")
	planTTL := fs.Duration("plan-ttl", mcpserver.DefaultPlanTTL, "")
	readTO := fs.Duration("read-timeout", mcpserver.DefaultReadTimeout, "")
	actTO := fs.Duration("act-timeout", mcpserver.DefaultActTimeout, "")
	list := fs.Bool("list-tools", false, "")
	showVersion := fs.Bool("version", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, helpText())
			return exitcode.OK
		}
		fmt.Fprintf(stderr, "winforge-mcp: %v\nTry: winforge-mcp --help\n", err)
		return exitcode.Usage
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "winforge-mcp: unexpected argument %q\nTry: winforge-mcp --help\n", fs.Arg(0))
		return exitcode.Usage
	}
	if *showVersion {
		fmt.Fprintf(stdout, "winforge-mcp %s\n", version)
		return exitcode.OK
	}
	mode, err := mcpserver.ParseMode(*modeS)
	if err != nil {
		fmt.Fprintf(stderr, "winforge-mcp: %v\n", err)
		return exitcode.Usage
	}
	for name, d := range map[string]time.Duration{"--plan-ttl": *planTTL, "--read-timeout": *readTO, "--act-timeout": *actTO} {
		if d <= 0 {
			fmt.Fprintf(stderr, "winforge-mcp: %s must be longer than zero\n", name)
			return exitcode.Usage
		}
	}

	env := newEnv(version)
	store := mcpserver.NewPlanStore(*planTTL, env.Now)
	reg, err := mcpserver.RealRegistry(env, store)
	if err != nil {
		fmt.Fprintf(stderr, "winforge-mcp: %v\n", err)
		return exitcode.Failures
	}
	if *list {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"mode": mode.String(), "tools": reg.ToolList(mode)})
		return exitcode.OK
	}

	srv, err := mcpserver.New(mcpserver.Options{
		Mode: mode, Registry: reg, Store: store, Env: env, Version: version,
		ReadTimeout: *readTO, ActTimeout: *actTO, Log: stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "winforge-mcp: %v\n", err)
		return exitcode.Failures
	}
	fmt.Fprintf(stderr, "winforge-mcp %s ready on stdio, mode %s\n", version, mode)
	if err := srv.Serve(ctx, stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "winforge-mcp: %v\n", err)
		return exitcode.Failures
	}
	return exitcode.OK
}

func helpText() string {
	s := help
	for _, e := range exitcode.Table {
		s += fmt.Sprintf("  %d  %s\n", e.Code, e.Meaning)
	}
	return s
}
