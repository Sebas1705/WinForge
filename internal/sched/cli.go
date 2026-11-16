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

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// Help is the text of `winforge-cli updates --help`.
const Help = `usage: winforge-cli updates [--mode notify|apply] [--scheduled]

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

What "apply" does and does not do:
  - It only updates apps of the WinForge catalog. It never runs recipes and never updates
    WinForge itself.
  - Updates you chose to ignore (skip a version, pause, never) are left alone. If the ignore
    list cannot be read, nothing is updated.
  - An app whose update needs administrator rights is reported, not updated: an unattended run
    never raises a permission prompt.
  - It never restarts the PC. An app that asks for a restart is only reported.
  - Only one run happens at a time, and a run stops by itself after 45 minutes.

Exit codes:
  0  the check (and any update) finished without problems
  1  some updates were tried and failed
  2  the command line was not understood
  3  another run is already in progress; nothing was done
  4  winget is missing or could not list the updates
  5  the ignore list is unreadable; nothing was done
  6  the run hit its time limit
`

// Deps lets tests run the command without a machine. A nil field gets the real thing.
type Deps struct {
	Runner *Runner
}

// RunCLI is `winforge-cli updates ...`: args are what follows "updates". It returns the exit code.
func RunCLI(args []string, out, errw io.Writer, d Deps) int {
	fs := flag.NewFlagSet("updates", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", string(Notify), "")
	scheduled := fs.Bool("scheduled", false, "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(errw, "%v\n\n%s", err, Help)
		return ExitUsage
	}
	if *help {
		fmt.Fprint(out, Help)
		return ExitOK
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(errw, "unexpected argument %q\n\n%s", fs.Arg(0), Help)
		return ExitUsage
	}
	if m := Mode(*mode); m != Notify && m != Apply {
		fmt.Fprintf(errw, "unknown mode %q: use notify or apply\n\n%s", *mode, Help)
		return ExitUsage
	}
	runner := d.Runner
	if runner == nil {
		r, err := NewRunner()
		if err != nil {
			fmt.Fprintln(errw, "winforge:", err)
			return ExitCheck
		}
		runner = &r
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := runner.Run(ctx, Options{Mode: Mode(*mode), Scheduled: *scheduled})
	if errors.Is(err, ErrBusy) {
		fmt.Fprintln(errw, "another update run is already in progress; nothing was done")
		return ExitBusy
	}
	if err != nil {
		fmt.Fprintln(errw, "winforge: could not store the run log:", err)
	}
	printReport(out, rep)
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
