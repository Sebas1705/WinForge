// Package cliout is what the winforge-cli subcommands share for unattended use: the global flags
// (--json, --quiet, --no-color, --yes, --non-interactive, --timeout) and the JSON envelope.
//
// The --json contract, for every subcommand:
//
//   - stdout is exactly ONE JSON document (UTF-8, no BOM, no ANSI, one line ending in "\n").
//     Human-readable progress and warnings go to stderr (nothing at all with --quiet).
//   - The document always starts with the fields of Envelope: schema, command, ok, exitCode and
//     error (null when there is none). The remaining, command-specific fields are listed in the
//     doc comment of each command's result type in cmd/winforge-cli/json.go (and for `updates` in
//     internal/sched/json.go).
//   - Field names are stable: a field is only ever added, never renamed or repurposed. Consumers
//     must ignore fields they do not know. Schema is bumped only for a breaking change.
//   - The process exit code is the same number as exitCode (see internal/exitcode).
package cliout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/exitcode"
)

// SchemaVersion is the version of the --json contract.
const SchemaVersion = 1

// Options are the global flags, accepted anywhere on the command line.
type Options struct {
	// JSON makes stdout one JSON document and sends human logs to stderr.
	JSON bool
	// Quiet drops progress and informational text (errors are still printed to stderr).
	Quiet bool
	// NoColor is accepted for scripts. WinForge's CLI never writes colour codes, so it changes nothing;
	// the NO_COLOR convention is honoured the same way.
	NoColor bool
	// NonInteractive (also --yes) promises nothing will ever wait for an answer. The CLI never asks a
	// question; the flag additionally makes `apply` refuse to start a plan that needs administrator
	// rights when this process is not elevated, instead of letting Windows raise a UAC prompt.
	NonInteractive bool
	// Timeout is the longest a command may run; zero means the command's own default.
	Timeout time.Duration
}

// FlagHelp is the "Global options" section of every --help.
const FlagHelp = `Global options (accepted by every command, anywhere on the line):
  --json             stdout is ONE JSON document (UTF-8, no colour codes); human logs go to stderr
  --quiet            no progress or informational text; errors still go to stderr
  --no-color         never write colour codes (WinForge never does; accepted for scripts)
  --yes, --non-interactive
                     never wait for an answer: nothing here ever asks, and a plan that needs
                     administrator rights is refused (exit 7) instead of raising a UAC prompt
  --timeout <time>   stop after this long, e.g. 90s or 30m (exit 6); honoured by every command
                     that can run long
`

// Split takes the global flags out of args and returns the rest in order. An invalid --timeout is
// an error (the caller reports a usage error).
func Split(args []string) (rest []string, o Options, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			o.JSON = true
		case a == "--quiet" || a == "-q":
			o.Quiet = true
		case a == "--no-color":
			o.NoColor = true
		case a == "--yes" || a == "--non-interactive":
			o.NonInteractive = true
		case a == "--timeout" || strings.HasPrefix(a, "--timeout="):
			v := strings.TrimPrefix(a, "--timeout=")
			if a == "--timeout" {
				if i+1 >= len(args) {
					return nil, o, fmt.Errorf("--timeout needs a duration such as 90s or 30m")
				}
				i++
				v = args[i]
			}
			d, perr := time.ParseDuration(v)
			if perr != nil || d <= 0 {
				return nil, o, fmt.Errorf("invalid --timeout %q: use a positive duration such as 90s or 30m", v)
			}
			o.Timeout = d
		default:
			rest = append(rest, a)
		}
	}
	return rest, o, nil
}

// Error is the error member of the envelope.
type Error struct {
	// Code is the stable machine word of the exit code (see exitcode.Table), e.g. "busy".
	Code string `json:"code"`
	// Detail is an optional finer code of the command (for updates: "no-winget", "check-failed", ...).
	Detail string `json:"detail,omitempty"`
	// Message is a sentence for a person reading a log. It is English and not meant to be parsed.
	Message string `json:"message"`
}

// Envelope is the start of every --json document.
type Envelope struct {
	Schema   int    `json:"schema"`
	Command  string `json:"command"`
	OK       bool   `json:"ok"` // true when exitCode is 0 or 8: the work was done
	ExitCode int    `json:"exitCode"`
	Error    *Error `json:"error"`
}

// NewEnvelope fills the envelope of a command that ended with code. msg is only used when the code
// is not a success; detail refines the code.
func NewEnvelope(command string, code int, detail, msg string) Envelope {
	e := Envelope{Schema: SchemaVersion, Command: command, ExitCode: code, OK: exitcode.Succeeded(code)}
	if code != exitcode.OK && code != exitcode.Reboot {
		e.Error = &Error{Code: exitcode.Name(code), Detail: detail, Message: msg}
	}
	return e
}

// Write writes v as one compact JSON document and a newline. HTML characters are not escaped, so
// the text is the same one a person sees.
func Write(w io.Writer, v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(b.Bytes())
	return err
}
