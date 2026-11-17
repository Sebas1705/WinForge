package sched

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// Exit codes of `winforge-cli updates`, documented in its --help. The numbers are shared by every
// subcommand and defined in internal/exitcode.
const (
	ExitOK       = exitcode.OK       // checked (and updated what it may); nothing went wrong
	ExitFailures = exitcode.Failures // some updates were tried and failed
	ExitUsage    = exitcode.Usage    // the command line was not understood
	ExitBusy     = exitcode.Busy     // another run is in progress: nothing was done
	ExitCheck    = exitcode.Prereq   // winget is missing or could not list the updates
	ExitSettings = exitcode.Data     // the ignore list is unreadable: nothing was done
	ExitTimeout  = exitcode.Timeout  // the run hit its time limit
	ExitReboot   = exitcode.Reboot   // updated fine, but an app asks for a restart
)

// Env is what a run needs from the machine; tests fill it with fakes.
type Env struct {
	Cat      *catalog.Catalog
	Upgrades func(context.Context) ([]system.Upgrade, error)
	Rules    func() (map[string]updates.Rule, error)
	Exec     install.Executor
	Now      func() time.Time
	Elevated func() bool
}

func (e Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// isSelf is true for anything that would update WinForge itself. WinForge is not a catalog app, and
// its own updater is never run unattended; this keeps it that way if the catalog ever gains an entry.
func isSelf(id, winget string) bool {
	return strings.Contains(strings.ToLower(id), "winforge") || strings.Contains(strings.ToLower(winget), "winforge")
}

// Execute looks for updates and, in Apply mode, installs the ones that may be installed without the
// person. It follows exactly the rules of the Updates page's "Update all":
//   - only catalog apps (install.UpgradePlan), never recipes and never WinForge itself;
//   - updates the person chose to ignore (skip, pause, never) are left alone, and an unreadable
//     ignore list stops the whole run rather than being read as "ignore nothing";
//   - an app whose update needs administrator rights is reported, not attempted: nothing unattended
//     may raise a UAC prompt;
//   - nothing is ever restarted; an app that asks for a restart is only reported.
//
// It does no I/O of its own beyond Env, so the rules are testable without a machine.
func Execute(ctx context.Context, env Env, mode Mode) Report {
	rep := Report{Started: env.now(), Mode: mode, Items: []Item{}}
	finish := func() Report {
		rep.Finished = env.now()
		rep.Fingerprint = rep.fingerprint()
		rep.Status = rep.Summary()
		return rep
	}
	if env.Cat == nil {
		rep.Error = ErrCodeNoCatalog
		return finish()
	}
	rules := map[string]updates.Rule{}
	if env.Rules != nil {
		var err error
		if rules, err = env.Rules(); err != nil {
			rep.Error = ErrCodeSettings
			return finish()
		}
	}
	ups, err := env.Upgrades(ctx)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
			rep.Error = ErrCodeTimeout
		case errors.Is(err, system.ErrNoWinget):
			rep.Error = ErrCodeNoWinget
		default:
			rep.Error = ErrCodeCheck
		}
		return finish()
	}

	now := env.now()
	current := map[string]string{}
	for _, u := range ups {
		current[u.ID] = u.Current
	}
	var queue []install.Step
	for _, s := range install.UpgradePlan(env.Cat, ups, nil).Steps {
		app := env.Cat.Apps[s.ID]
		it := Item{ID: s.ID, Name: app.Name, From: current[app.Winget], To: s.Version}
		switch r, hidden := rules[s.ID]; {
		case hidden && r.Hides(s.Version, now):
			it.Result, it.Reason = ResultIgnored, string(r.Mode)
		case isSelf(s.ID, app.Winget):
			it.Result, it.Reason = ResultIgnored, "self"
		case s.Admin && !(env.Elevated != nil && env.Elevated()):
			it.Result = ResultNeedsAdmin
		case mode == Apply:
			it.Result = ResultSkipped // until the run says otherwise
			queue = append(queue, s)
		default:
			it.Result = ResultFound
		}
		rep.Items = append(rep.Items, it)
	}
	sort.SliceStable(rep.Items, func(i, j int) bool {
		return strings.ToLower(rep.Items[i].Name) < strings.ToLower(rep.Items[j].Name)
	})

	if mode == Apply && len(queue) > 0 && env.Exec != nil {
		at := map[string]int{}
		for i, it := range rep.Items {
			at[it.ID] = i
		}
		install.Run(ctx, env.Cat, install.Plan{Steps: queue}, env.Exec, func(e install.Event) {
			i, ok := at[e.Step.ID]
			if !ok {
				return
			}
			it := &rep.Items[i]
			switch e.Status {
			case install.StatusOK:
				it.Result, it.Reboot = ResultUpdated, e.Reason == install.ReasonReboot
			case install.StatusFailed:
				if e.Reason == install.ReasonAdmin {
					it.Result = ResultNeedsAdmin
				} else {
					it.Result, it.Reason = ResultFailed, e.Reason
				}
			case install.StatusSkipped:
				it.Result, it.Reason = ResultSkipped, e.Err
			}
		})
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			rep.Error = ErrCodeTimeout
		}
	}
	return finish()
}

// ExitCode says how a run ended, for the command line.
func (r Report) ExitCode() int {
	switch r.Error {
	case "":
	case ErrCodeTimeout:
		return ExitTimeout
	case ErrCodeSettings:
		return ExitSettings
	default:
		return ExitCheck
	}
	if r.Count(ResultFailed) > 0 {
		return ExitFailures
	}
	if r.Restart() {
		return ExitReboot
	}
	return ExitOK
}

// Runner is a complete run: one at a time, with a time limit, a log and a toast.
type Runner struct {
	Env     Env
	Logs    LogStore
	Lock    Lock
	Sys     System        // raises the toast; nil means none
	Lang    func() string // the toast's language
	Timeout time.Duration // zero means RunTimeout
}

// Options are the choices of one run.
type Options struct {
	Mode Mode
	// Scheduled is the unattended run: it writes the log and raises the toast. Without it (a check from
	// the Updates page, or a command typed in a terminal) nothing is stored and nothing pops up.
	Scheduled bool
}

// Run takes the lock (ErrBusy when another run holds it), executes with the time limit and, for a
// scheduled run, logs it and raises the toast when there is something to say.
func (r Runner) Run(ctx context.Context, o Options) (Report, error) {
	release, err := r.Lock.Acquire()
	if err != nil {
		return Report{}, err
	}
	defer release()
	limit := r.Timeout
	if limit <= 0 {
		limit = RunTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()

	rep := Execute(ctx, r.Env, o.Mode)
	rep.Scheduled = o.Scheduled
	if !o.Scheduled {
		return rep, nil
	}
	if toast, ok := r.toast(rep); ok {
		// The same news is not announced twice in a row: a daily check that keeps finding the same
		// three updates would otherwise become a daily nag.
		if rep.fingerprint() != r.Logs.lastNotified() {
			if r.Sys != nil && r.Sys.Toast(toast) == nil {
				rep.Notified = true
			}
		}
	}
	if err := r.Logs.Save(rep); err != nil {
		return rep, err
	}
	return rep, nil
}

func (r Runner) toast(rep Report) (Toast, bool) {
	lang := "en"
	if r.Lang != nil {
		lang = NormalizeLanguage(r.Lang())
	}
	return Compose(rep, lang)
}
