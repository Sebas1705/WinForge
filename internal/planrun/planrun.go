// Package planrun runs an install plan the way the unattended entry points do (winforge-cli apply
// and the MCP server): it checks what must hold before anything starts (winget, administrator
// rights, the one-run lock), runs the plan through install.Run, turns the events into one result per
// step and, when asked, rescans once to say which version each step ended on. It prints nothing and
// knows no exit codes: callers word and map the Report themselves.
package planrun

import (
	"context"
	"errors"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

// Refusal reasons: the run was not started and every step is "skipped" with this reason.
const (
	RefusedPrerequisite = "prerequisite" // winget is missing and a step needs it
	RefusedAdmin        = "admin"        // administrator rights are needed and the run has none
	RefusedBusy         = "busy"         // another run holds the lock
)

// Step is the outcome of one planned step, in plan order.
type Step struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Result  string `json:"result"` // ok | failed | skipped
	Reason  string `json:"reason"`
	Message string `json:"message"`
	From    string `json:"from"`
	To      string `json:"to"`
	Reboot  bool   `json:"reboot"`
	Admin   bool   `json:"admin"`
}

// Report is what a run did.
type Report struct {
	Steps          []Step
	Counts         map[string]int // ok, failed, skipped
	RebootRequired bool
	Failed         []string // ids of the steps that failed
	// Refused is one of the Refused* reasons when the run never started ("" otherwise);
	// RefusedMessage is a ready English sentence for it.
	Refused        string
	RefusedMessage string
	// After is the rescan taken once the run ended (nil when none was asked for or nothing succeeded).
	After map[string]system.Installed
}

// Options are the machine and the policy of one run.
type Options struct {
	Exec install.Executor
	// Elevated says whether this process has administrator rights; nil means no.
	Elevated func() bool
	// RequireAdmin refuses a plan that needs administrator rights when the process is not elevated,
	// instead of letting Windows raise a prompt nobody can answer.
	RequireAdmin bool
	// LockPath is the lock shared by every unattended run; nil skips locking.
	LockPath func() (string, error)
	// Installed is the scan the plan was built from; it gives each step's "from" version.
	Installed map[string]system.Installed
	// WingetMissing is true when the scan found no winget; steps that are not recipes are then
	// refused with WingetMessage.
	WingetMissing bool
	WingetMessage string
	// Detect rescans the PC after the run to fill in each step's "to" version; nil skips the rescan.
	Detect func() (map[string]system.Installed, error)
	// OnWarn hears about a problem that did not stop the run (the lock could not be taken).
	OnWarn func(string)
	// OnStart is called once the run is about to start (after every check passed).
	OnStart func()
	// OnEvent hears every event of the run, after the step table is updated.
	OnEvent func(install.Event)
}

func stepKey(s install.Step) string { return string(s.Kind) + "/" + s.ID }

// Run executes the plan. It never returns an error: a refusal, a failure and a cancellation are all
// in the Report (cancellation is ctx.Err() on the caller's side).
func Run(ctx context.Context, cat *catalog.Catalog, plan install.Plan, o Options) *Report {
	rep := &Report{Steps: []Step{}, Counts: map[string]int{"ok": 0, "failed": 0, "skipped": 0}}
	at := map[string]int{}
	needsWinget := false
	for _, s := range plan.Steps {
		at[stepKey(s)] = len(rep.Steps)
		from := ""
		if s.Kind != install.StepApp && s.Kind != install.StepRecipe {
			from = o.Installed[s.ID].Version
		}
		rep.Steps = append(rep.Steps, Step{ID: s.ID, Kind: string(s.Kind), Name: s.Name, Result: "skipped", From: from, To: from, Admin: s.Admin})
		needsWinget = needsWinget || s.Kind != install.StepRecipe
	}
	count := func() {
		rep.Counts = map[string]int{"ok": 0, "failed": 0, "skipped": 0}
		for _, st := range rep.Steps {
			rep.Counts[st.Result]++
		}
	}
	refuse := func(reason, msg string) *Report {
		for i := range rep.Steps {
			rep.Steps[i].Reason = reason
		}
		rep.Refused, rep.RefusedMessage = reason, msg
		count()
		return rep
	}

	if o.WingetMissing && needsWinget {
		return refuse(RefusedPrerequisite, o.WingetMessage+"; nothing was done")
	}
	if o.RequireAdmin && plan.NeedsAdmin && !(o.Elevated != nil && o.Elevated()) {
		return refuse(RefusedAdmin, "this plan needs administrator rights and this run has none; nothing was done (start winforge-cli from an elevated terminal)")
	}
	if o.LockPath != nil {
		if p, err := o.LockPath(); err == nil {
			release, lerr := sched.Lock{Path: p}.Acquire()
			if errors.Is(lerr, sched.ErrBusy) {
				return refuse(RefusedBusy, "another WinForge run is already in progress; nothing was done")
			}
			if lerr == nil {
				defer release()
			} else if o.OnWarn != nil {
				o.OnWarn("could not take the run lock: " + lerr.Error())
			}
		}
	}
	if o.OnStart != nil {
		o.OnStart()
	}

	rep.Failed = install.Run(ctx, cat, plan, o.Exec, func(e install.Event) {
		if i, ok := at[stepKey(e.Step)]; ok {
			st := &rep.Steps[i]
			switch e.Status {
			case install.StatusOK:
				st.Result, st.Reboot = "ok", e.Reason == install.ReasonReboot
			case install.StatusFailed:
				st.Result, st.Reason, st.Message = "failed", e.Reason, e.Err
			case install.StatusSkipped:
				st.Result, st.Message = "skipped", e.Err
				st.Reason = "dependency"
				if e.Err == "cancelled" {
					st.Reason = "cancelled"
				}
			}
		}
		if o.OnEvent != nil {
			o.OnEvent(e)
		}
	})
	if o.Detect != nil && ctx.Err() == nil {
		fillAfter(rep, plan, at, o.Detect)
	}
	count()
	for _, st := range rep.Steps {
		rep.RebootRequired = rep.RebootRequired || st.Reboot
	}
	return rep
}

// fillAfter rescans once to report the version each step ended with. A failed scan only leaves the
// "to" of an app step empty.
func fillAfter(rep *Report, plan install.Plan, at map[string]int, detect func() (map[string]system.Installed, error)) {
	done := false
	for _, st := range rep.Steps {
		done = done || st.Result == "ok"
	}
	if !done {
		return
	}
	after, err := detect()
	if err != nil && len(after) == 0 {
		return
	}
	rep.After = after
	for _, s := range plan.Steps {
		st := &rep.Steps[at[stepKey(s)]]
		if st.Result != "ok" {
			continue
		}
		switch s.Kind {
		case install.StepUninstall, install.StepRecipe:
			st.To = ""
		default:
			st.To = after[s.ID].Version
			if st.To == "" {
				st.To = s.Version
			}
		}
	}
}
