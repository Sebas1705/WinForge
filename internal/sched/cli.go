package sched

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/cliout"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// Help is the text of `winforge-cli updates --help`.
var Help = `usage: winforge-cli updates [--mode notify|apply] [--scheduled] [--json] [--quiet] [--timeout <time>]

Looks for updates to the catalog apps installed on this PC, with the same rules as the
Updates page of WinForge.

  --mode notify    only check and report (the default)
  --mode apply     also update the apps that may be updated without you
  --scheduled      the unattended run the Windows scheduled task "WinForge updates" starts:
                   it also keeps a log of the run (the last 30 are kept, in
                   %AppData%\WinForge\auto-updates\runs) and raises a Windows notification
                   when there is something to tell. Without it nothing is stored and no
                   notification is shown.
  -h, --help       show this text

` + cliout.FlagHelp + `
What "apply" does and does not do:
  - It only updates apps of the WinForge catalog. It never runs recipes and never updates
    WinForge itself.
  - Updates you chose to ignore (skip, pause, never) are left alone. If the ignore
    list cannot be read, nothing is updated.
  - An app whose update needs administrator rights is reported, not updated: an unattended run
    never raises a permission prompt.
  - It never restarts the PC. An app that asks for a restart is only reported.
  - Only one run happens at a time, and a run stops by itself after 45 minutes (--timeout
    can shorten or lengthen that).

With --json, stdout is one document: schema, command, ok, exitCode, error, mode, scheduled,
started, finished, restartRequired, counts and items (id, name, from, to, result, reason,
reboot per app).

Exit codes:
  0  the check (and any update) finished without problems
  1  some updates were tried and failed
  2  the command line was not understood
  3  another run is already in progress; nothing was done
  4  winget is missing or could not list the updates
  5  the ignore list is unreadable; nothing was done
  6  the run hit its time limit
  7  not used here: an app that needs administrator rights is reported, never attempted
  8  everything worked, but an updated app asks for a restart (the PC is never restarted)
`

// Deps lets tests run the command without a machine. A nil field gets the real thing.
type Deps struct {
	Runner *Runner
}

// RunCLI is `winforge-cli updates ...`: args are what follows "updates". It returns the exit code.
// Global flags (--json, --quiet, ...) are taken out of args first.
func RunCLI(args []string, out, errw io.Writer, d Deps) int {
	rest, o, err := cliout.Split(args)
	if err != nil {
		fmt.Fprintf(errw, "%v\n\n%s", err, Help)
		if wantsJSON(args) {
			_ = cliout.Write(out, jsonFailure(exitcode.Usage, err.Error()))
		}
		return ExitUsage
	}
	return RunCLIWith(rest, o, out, errw, d)
}

// wantsJSON is used when the flags could not be parsed at all.
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

// RunCLIWith is RunCLI after the global flags have been read: args holds only the command's own.
func RunCLIWith(args []string, o cliout.Options, out, errw io.Writer, d Deps) int {
	fail := func(code int, detail, msg string, showHelp bool) int {
		if showHelp {
			fmt.Fprintf(errw, "%s\n\n%s", msg, Help)
		} else {
			fmt.Fprintln(errw, msg)
		}
		if o.JSON {
			f := jsonFailure(code, msg)
			f.Error.Detail = detail
			_ = cliout.Write(out, f)
		}
		return code
	}
	fs := flag.NewFlagSet("updates", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", string(Notify), "")
	scheduled := fs.Bool("scheduled", false, "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		return fail(ExitUsage, "", err.Error(), true)
	}
	if *help {
		fmt.Fprint(out, Help)
		return ExitOK
	}
	if fs.NArg() > 0 {
		return fail(ExitUsage, "", fmt.Sprintf("unexpected argument %q", fs.Arg(0)), true)
	}
	if m := Mode(*mode); m != Notify && m != Apply {
		return fail(ExitUsage, "", fmt.Sprintf("unknown mode %q: use notify or apply", *mode), true)
	}
	runner := d.Runner
	if runner == nil {
		r, err := NewRunner()
		if err != nil {
			return fail(ExitCheck, ErrCodeNoCatalog, "winforge: "+err.Error(), false)
		}
		runner = &r
	}
	if o.Timeout > 0 {
		r := *runner
		r.Timeout = o.Timeout
		runner = &r
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := runner.Run(ctx, Options{Mode: Mode(*mode), Scheduled: *scheduled})
	if errors.Is(err, ErrBusy) {
		return fail(ExitBusy, "", "another update run is already in progress; nothing was done", false)
	}
	if err != nil && !o.Quiet {
		fmt.Fprintln(errw, "winforge: could not store the run log:", err)
	}
	// With --json the human report goes to stderr so stdout stays one document.
	human := out
	if o.JSON {
		human = errw
	}
	if !(o.JSON && o.Quiet) {
		printReport(human, rep)
	}
	if o.JSON {
		_ = cliout.Write(out, newJSONReport(rep))
	}
	return rep.ExitCode()
}

func printReport(w io.Writer, r Report) {
	if r.Error != "" {
		fmt.Fprintf(w, "run problem: %s\n", r.Error)
	}
	for _, it := range r.Items {
		extra := ""
		if it.Reason != "" {
			extra = " (" + it.Reason + ")"
		}
		if it.Reboot {
			extra += " (restart needed)"
		}
		fmt.Fprintf(w, "%-12s %-28s %s -> %s%s\n", it.Result, it.Name, it.From, it.To, extra)
	}
	fmt.Fprintf(w, "\n%d to update, %d updated, %d need you, %d failed, %d ignored\n",
		r.Count(ResultFound), r.Count(ResultUpdated), r.Count(ResultNeedsAdmin), r.Count(ResultFailed), r.Count(ResultIgnored))
}

// jsonReport is the --json document of `winforge-cli updates`:
//
//	schema, command ("updates"), ok, exitCode, error  the common envelope (see internal/cliout)
//	mode             "notify" | "apply"
//	scheduled        true for the unattended scheduled run
//	started, finished  RFC 3339 times
//	restartRequired  true when an updated app asks for a restart (exit code 8)
//	counts           {found, updated, failed, ignored, needs-admin, skipped}: how many items have each result
//	items[]          {id, name, from, to, result, reason?, reboot?}: from and to are the version before
//	                 and after (to is the version available while nothing was updated); result is one of
//	                 found | updated | failed | ignored | needs-admin | skipped; reason explains failed
//	                 (install.Explain's word) and ignored (the ignore mode, or "self"); reboot is true when
//	                 that app asks for a restart
//
// When the run itself could not work, error.detail is one of no-winget, check-failed,
// settings-damaged, timeout or no-catalog and items is empty.
type jsonReport struct {
	cliout.Envelope
	Mode            Mode           `json:"mode"`
	Scheduled       bool           `json:"scheduled"`
	Started         time.Time      `json:"started"`
	Finished        time.Time      `json:"finished"`
	RestartRequired bool           `json:"restartRequired"`
	Counts          map[string]int `json:"counts"`
	Items           []Item         `json:"items"`
}

func newJSONReport(r Report) jsonReport {
	code := r.ExitCode()
	msg := ""
	if r.Error != "" {
		msg = "the run could not finish: " + r.Error
	} else if code == ExitFailures {
		msg = "some updates failed"
	}
	env := cliout.NewEnvelope("updates", code, r.Error, msg)
	items := r.Items
	if items == nil {
		items = []Item{}
	}
	counts := map[string]int{}
	for _, res := range []string{ResultFound, ResultUpdated, ResultFailed, ResultIgnored, ResultNeedsAdmin, ResultSkipped} {
		counts[res] = r.Count(res)
	}
	return jsonReport{Envelope: env, Mode: r.Mode, Scheduled: r.Scheduled, Started: r.Started, Finished: r.Finished,
		RestartRequired: r.Restart(), Counts: counts, Items: items}
}

// jsonFailure is the document of a command that did nothing.
func jsonFailure(code int, msg string) jsonReport {
	return jsonReport{Envelope: cliout.NewEnvelope("updates", code, "", msg), Items: []Item{}, Counts: map[string]int{}}
}

// NewRunner is the real thing: this PC's catalog, winget, ignore list, log folder and notifications.
func NewRunner() (Runner, error) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		return Runner{}, err
	}
	dir, err := DefaultDir()
	if err != nil {
		return Runner{}, err
	}
	store, err := updates.DefaultStore()
	if err != nil {
		return Runner{}, err
	}
	return Runner{
		Env: Env{
			Cat: cat, Upgrades: system.WingetUpgrades, Rules: store.Load,
			Exec: install.OSExecutor{}, Elevated: system.IsElevated,
		},
		Logs: LogStore{Dir: dir},
		Lock: Lock{Path: dir + string(os.PathSeparator) + "run.lock"},
		Sys:  NewSystem(),
		Lang: func() string { return Language(dir, SystemLanguage) },
	}, nil
}

// IsUpdatesCommand tells whether a command line is the headless update run, so the desktop program
// can answer it without opening a window.
func IsUpdatesCommand(args []string) bool {
	return len(args) > 1 && strings.EqualFold(args[1], "updates")
}
